package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/ggfarmco/kg/internal/graph"
)

func (s *Store) CreateEdge(ctx context.Context, e *graph.Edge) error {
	props, err := json.Marshal(e.Properties)
	if err != nil {
		return fmt.Errorf("marshal properties: %w", err)
	}
	return s.inTxOrConn(ctx, func(ctx context.Context) error {
		q := New(s.conn(ctx))
		id, err := q.CreateEdge(ctx, CreateEdgeParams{
			SourceID:   string(e.SourceID),
			TargetID:   string(e.TargetID),
			Type:       e.Type,
			Properties: string(props),
			CreatedAt:  e.CreatedAt.UnixMilli(),
		})
		if err != nil {
			return mapSQLiteErr(err, "edge")
		}
		e.ID = graph.EdgeID(id)
		e.Revision = 1
		rev := int64(1)
		return q.AppendChange(ctx, AppendChangeParams{
			Entity:   "edge",
			EntityID: strconv.FormatInt(id, 10),
			Op:       "create",
			Revision: &rev,
			At:       e.CreatedAt.UnixMilli(),
		})
	})
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
		return q.AppendChange(ctx, AppendChangeParams{
			Entity:   "edge",
			EntityID: strconv.FormatInt(int64(id), 10),
			Op:       "delete",
			Revision: nil,
			At:       time.Now().UnixMilli(),
		})
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
	var props map[string]any
	if err := json.Unmarshal([]byte(r.Properties), &props); err != nil {
		return nil, fmt.Errorf("unmarshal properties: %w", err)
	}
	return &graph.Edge{
		ID:         graph.EdgeID(r.ID),
		SourceID:   graph.NodeID(r.SourceID),
		TargetID:   graph.NodeID(r.TargetID),
		Type:       r.Type,
		Properties: props,
		Revision:   r.Revision,
		CreatedAt:  time.UnixMilli(r.CreatedAt),
	}, nil
}
