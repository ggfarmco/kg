package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/ggfarmco/kg/internal/graph"
)

func (s *Store) CreateDomain(ctx context.Context, d graph.Domain) error {
	return s.inTxOrConn(ctx, func(ctx context.Context) error {
		layers, err := json.Marshal(d.Layers)
		if err != nil {
			return fmt.Errorf("marshal layers: %w", err)
		}
		q := New(s.conn(ctx))
		if err := q.CreateDomain(ctx, CreateDomainParams{
			ID:          string(d.ID),
			Description: nullStringPtr(d.Description),
			Layers:      string(layers),
			CreatedAt:   d.CreatedAt.UnixMilli(),
		}); err != nil {
			return mapSQLiteErr(err, "domain")
		}
		rev := int64(1)
		return q.AppendChange(ctx, AppendChangeParams{
			Entity:   "domain",
			EntityID: string(d.ID),
			Op:       "create",
			Revision: &rev,
			At:       d.CreatedAt.UnixMilli(),
		})
	})
}

func (s *Store) GetDomain(ctx context.Context, id graph.DomainID) (*graph.Domain, error) {
	q := New(s.conn(ctx))
	row, err := q.GetDomain(ctx, string(id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, graph.ErrDomainNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite: get domain: %w", err)
	}
	return decodeDomain(row.ID, row.Description, row.Layers, row.Revision, row.CreatedAt)
}

func (s *Store) ListDomains(ctx context.Context) ([]graph.Domain, error) {
	q := New(s.conn(ctx))
	rows, err := q.ListDomains(ctx)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list domains: %w", err)
	}
	out := make([]graph.Domain, 0, len(rows))
	for _, r := range rows {
		d, err := decodeDomain(r.ID, r.Description, r.Layers, r.Revision, r.CreatedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, nil
}

func (s *Store) DeleteDomain(ctx context.Context, id graph.DomainID) error {
	return s.inTxOrConn(ctx, func(ctx context.Context) error {
		q := New(s.conn(ctx))
		if _, err := q.GetDomain(ctx, string(id)); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return graph.ErrDomainNotFound
			}
			return fmt.Errorf("sqlite: get domain: %w", err)
		}
		if err := q.DeleteDomain(ctx, string(id)); err != nil {
			return mapSQLiteErr(err, "domain")
		}
		return q.AppendChange(ctx, AppendChangeParams{
			Entity:   "domain",
			EntityID: string(id),
			Op:       "delete",
			Revision: nil,
			At:       time.Now().UnixMilli(),
		})
	})
}

func decodeDomain(id string, desc *string, layersJSON string, rev, createdAt int64) (*graph.Domain, error) {
	var layers []string
	if err := json.Unmarshal([]byte(layersJSON), &layers); err != nil {
		return nil, fmt.Errorf("unmarshal layers: %w", err)
	}
	d := &graph.Domain{
		ID:       graph.DomainID(id),
		Layers:   layers,
		Revision: rev,
		CreatedAt: time.UnixMilli(createdAt),
	}
	if desc != nil {
		d.Description = *desc
	}
	return d, nil
}

func nullStringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func mapSQLiteErr(err error, entity string) error {
	var se *sqlite.Error
	if !errors.As(err, &se) {
		return err
	}
	if se.Code() == sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY || se.Code() == sqlite3.SQLITE_CONSTRAINT_TRIGGER {
		return graph.ErrHasDependents
	}
	if se.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE || se.Code() == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY {
		switch entity {
		case "domain":
			return graph.ErrDomainAlreadyExists
		case "node":
			return graph.ErrNodeAlreadyExists
		case "edge":
			return graph.ErrEdgeAlreadyExists
		}
	}
	return err
}
