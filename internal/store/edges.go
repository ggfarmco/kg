package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ggfarmco/kg/internal/graph"
)

func (s *Store) UpsertEdge(ctx context.Context, e graph.Edge) (graph.EdgeID, error) {
	props, err := encodeNamespacedProps(e.Properties)
	if err != nil {
		return 0, err
	}
	var id int64
	err = s.inTxOrConn(ctx, func(ctx context.Context) error {
		q := New(s.conn(ctx))
		newID, qerr := q.UpsertEdge(ctx, UpsertEdgeParams{
			SourceID:   string(e.SourceID),
			TargetID:   string(e.TargetID),
			Type:       e.Type,
			Properties: props,
			CreatedAt:  e.CreatedAt.UnixMilli(),
		})
		if qerr != nil {
			return mapSQLiteErr(qerr, "edge")
		}
		id = newID
		return nil
	})
	return graph.EdgeID(id), err
}

func (s *Store) UpdateEdgeProperties(ctx context.Context, id graph.EdgeID, props map[graph.SourceID]map[string]any) error {
	encoded, err := encodeNamespacedProps(props)
	if err != nil {
		return err
	}
	q := New(s.conn(ctx))
	return q.UpdateEdgeProperties(ctx, UpdateEdgePropertiesParams{ID: int64(id), Properties: encoded})
}

func (s *Store) GetEdge(ctx context.Context, id graph.EdgeID) (*graph.Edge, error) {
	q := New(s.conn(ctx))
	row, err := q.GetEdge(ctx, int64(id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, graph.ErrEdgeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite: get edge: %w", err)
	}
	return decodeEdge(row)
}

func (s *Store) DeleteEdge(ctx context.Context, id graph.EdgeID) error {
	return s.inTxOrConn(ctx, func(ctx context.Context) error {
		q := New(s.conn(ctx))
		if _, err := q.GetEdge(ctx, int64(id)); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return graph.ErrEdgeNotFound
			}
			return fmt.Errorf("sqlite: get edge: %w", err)
		}
		if err := q.DeleteEdge(ctx, int64(id)); err != nil {
			return mapSQLiteErr(err, "edge")
		}
		return nil
	})
}

func (s *Store) EdgesFrom(ctx context.Context, src graph.NodeID, types []string) ([]graph.Edge, error) {
	q := New(s.conn(ctx))
	if len(types) == 0 {
		rows, err := q.EdgesFromAll(ctx, string(src))
		if err != nil {
			return nil, fmt.Errorf("sqlite: edges from: %w", err)
		}
		return decodeEdges(rows)
	}
	rows, err := q.EdgesFromTyped(ctx, EdgesFromTypedParams{SourceID: string(src), Types: types})
	if err != nil {
		return nil, fmt.Errorf("sqlite: edges from typed: %w", err)
	}
	return decodeEdges(rows)
}

func (s *Store) EdgesTo(ctx context.Context, dst graph.NodeID, types []string) ([]graph.Edge, error) {
	q := New(s.conn(ctx))
	if len(types) == 0 {
		rows, err := q.EdgesToAll(ctx, string(dst))
		if err != nil {
			return nil, fmt.Errorf("sqlite: edges to: %w", err)
		}
		return decodeEdges(rows)
	}
	rows, err := q.EdgesToTyped(ctx, EdgesToTypedParams{TargetID: string(dst), Types: types})
	if err != nil {
		return nil, fmt.Errorf("sqlite: edges to typed: %w", err)
	}
	return decodeEdges(rows)
}

func decodeEdges(rows []Edge) ([]graph.Edge, error) {
	out := make([]graph.Edge, 0, len(rows))
	for _, r := range rows {
		e, err := decodeEdge(r)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, nil
}

func decodeEdge(r Edge) (*graph.Edge, error) {
	props, err := decodeNamespacedProps(r.Properties)
	if err != nil {
		return nil, err
	}
	return &graph.Edge{
		ID: graph.EdgeID(r.ID), SourceID: graph.NodeID(r.SourceID),
		TargetID: graph.NodeID(r.TargetID), Type: r.Type,
		Properties: props, Revision: r.Revision,
		CreatedAt: time.UnixMilli(r.CreatedAt),
	}, nil
}
