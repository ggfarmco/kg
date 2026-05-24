package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ggfarmco/kg/internal/graph"
)

func (s *Store) UpsertSource(ctx context.Context, src graph.Source) error {
	return s.inTxOrConn(ctx, func(ctx context.Context) error {
		q := New(s.conn(ctx))
		return q.UpsertSource(ctx, UpsertSourceParams{
			ID:          string(src.ID),
			Description: nullStringPtr(src.Description),
			FirstSeen:   src.FirstSeen.UnixMilli(),
			LastSeen:    src.LastSeen.UnixMilli(),
		})
	})
}

func (s *Store) GetSource(ctx context.Context, id graph.SourceID) (*graph.Source, error) {
	q := New(s.conn(ctx))
	row, err := q.GetSource(ctx, string(id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, graph.ErrSourceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite: get source: %w", err)
	}
	return decodeSource(row.ID, row.Description, row.FirstSeen, row.LastSeen), nil
}

func (s *Store) ListSources(ctx context.Context) ([]graph.Source, error) {
	q := New(s.conn(ctx))
	rows, err := q.ListSources(ctx)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list sources: %w", err)
	}
	out := make([]graph.Source, 0, len(rows))
	for _, r := range rows {
		out = append(out, *decodeSource(r.ID, r.Description, r.FirstSeen, r.LastSeen))
	}
	return out, nil
}

func (s *Store) UpdateSource(ctx context.Context, src graph.Source) error {
	q := New(s.conn(ctx))
	return q.UpdateSource(ctx, UpdateSourceParams{
		ID:          string(src.ID),
		Description: nullStringPtr(src.Description),
	})
}

func (s *Store) DeleteSource(ctx context.Context, id graph.SourceID) error {
	q := New(s.conn(ctx))
	if err := q.DeleteSource(ctx, string(id)); err != nil {
		if isFKViolation(err) {
			return graph.ErrSourceHasDependents
		}
		return fmt.Errorf("sqlite: delete source: %w", err)
	}
	return nil
}

func decodeSource(id string, desc *string, firstSeen, lastSeen int64) *graph.Source {
	d := ""
	if desc != nil {
		d = *desc
	}
	return &graph.Source{
		ID:        graph.SourceID(id),
		Description: d,
		FirstSeen: time.UnixMilli(firstSeen),
		LastSeen:  time.UnixMilli(lastSeen),
	}
}

func isFKViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "FOREIGN KEY constraint failed")
}
