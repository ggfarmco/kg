package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ggfarmco/kg/internal/graph"
)

func (s *Store) CreateNode(ctx context.Context, n graph.Node) error {
	props, err := json.Marshal(n.Properties)
	if err != nil {
		return fmt.Errorf("marshal properties: %w", err)
	}
	return s.inTxOrConn(ctx, func(ctx context.Context) error {
		q := New(s.conn(ctx))
		if err := q.CreateNode(ctx, CreateNodeParams{
			ID:         string(n.ID),
			Domain:     string(n.Domain),
			Layer:      n.Layer,
			Name:       n.Name,
			ParentID:   nodeIDPtr(n.ParentID),
			Summary:    nullStringPtr(n.Summary),
			Properties: string(props),
			CreatedAt:  n.CreatedAt.UnixMilli(),
			UpdatedAt:  n.UpdatedAt.UnixMilli(),
		}); err != nil {
			return mapSQLiteErr(err, "node")
		}
		rev := int64(1)
		return q.AppendChange(ctx, AppendChangeParams{
			Entity:   "node",
			EntityID: string(n.ID),
			Op:       "create",
			Revision: &rev,
			At:       n.CreatedAt.UnixMilli(),
		})
	})
}

func (s *Store) GetNode(ctx context.Context, id graph.NodeID) (*graph.Node, error) {
	q := New(s.conn(ctx))
	row, err := q.GetNode(ctx, string(id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, graph.ErrNodeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite: get node: %w", err)
	}
	return decodeNode(row.ID, row.Domain, row.Layer, row.Name, row.ParentID, row.Summary, row.Properties, row.Revision, row.CreatedAt, row.UpdatedAt)
}

func (s *Store) ListNodes(ctx context.Context, f graph.NodeFilter) ([]graph.Node, error) {
	q := New(s.conn(ctx))
	rows, err := q.ListNodes(ctx, ListNodesParams{
		DomainFilter: string(f.Domain),
		LayerFilter:  f.Layer,
		Lim:          int64(f.Limit),
	})
	if err != nil {
		return nil, fmt.Errorf("sqlite: list nodes: %w", err)
	}
	out := make([]graph.Node, 0, len(rows))
	for _, r := range rows {
		n, err := decodeNode(r.ID, r.Domain, r.Layer, r.Name, r.ParentID, r.Summary, r.Properties, r.Revision, r.CreatedAt, r.UpdatedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, *n)
	}
	return out, nil
}

func (s *Store) ChildrenOf(ctx context.Context, parentID graph.NodeID) ([]graph.Node, error) {
	q := New(s.conn(ctx))
	pid := string(parentID)
	rows, err := q.ChildrenOf(ctx, &pid)
	if err != nil {
		return nil, fmt.Errorf("sqlite: children of: %w", err)
	}
	out := make([]graph.Node, 0, len(rows))
	for _, r := range rows {
		n, err := decodeNode(r.ID, r.Domain, r.Layer, r.Name, r.ParentID, r.Summary, r.Properties, r.Revision, r.CreatedAt, r.UpdatedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, *n)
	}
	return out, nil
}

func (s *Store) UpdateNode(ctx context.Context, n graph.Node) error {
	props, err := json.Marshal(n.Properties)
	if err != nil {
		return fmt.Errorf("marshal properties: %w", err)
	}
	return s.inTxOrConn(ctx, func(ctx context.Context) error {
		q := New(s.conn(ctx))
		if _, err := q.GetNode(ctx, string(n.ID)); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return graph.ErrNodeNotFound
			}
			return fmt.Errorf("sqlite: get node: %w", err)
		}
		if err := q.UpdateNode(ctx, UpdateNodeParams{
			ID:         string(n.ID),
			Name:       n.Name,
			Summary:    nullStringPtr(n.Summary),
			Properties: string(props),
			UpdatedAt:  n.UpdatedAt.UnixMilli(),
		}); err != nil {
			return mapSQLiteErr(err, "node")
		}
		rev, err := q.GetNodeRevision(ctx, string(n.ID))
		if err != nil {
			return fmt.Errorf("sqlite: get node revision: %w", err)
		}
		return q.AppendChange(ctx, AppendChangeParams{
			Entity:   "node",
			EntityID: string(n.ID),
			Op:       "update",
			Revision: &rev,
			At:       n.UpdatedAt.UnixMilli(),
		})
	})
}

func (s *Store) DeleteNode(ctx context.Context, id graph.NodeID) error {
	return s.inTxOrConn(ctx, func(ctx context.Context) error {
		q := New(s.conn(ctx))
		if _, err := q.GetNode(ctx, string(id)); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return graph.ErrNodeNotFound
			}
			return fmt.Errorf("sqlite: get node: %w", err)
		}
		if err := q.DeleteNode(ctx, string(id)); err != nil {
			return mapSQLiteErr(err, "node")
		}
		return q.AppendChange(ctx, AppendChangeParams{
			Entity:   "node",
			EntityID: string(id),
			Op:       "delete",
			Revision: nil,
			At:       time.Now().UnixMilli(),
		})
	})
}

func decodeNode(id, domain, layer, name string, parent *string, summary *string, propsJSON string, rev, createdAt, updatedAt int64) (*graph.Node, error) {
	var props map[string]any
	if err := json.Unmarshal([]byte(propsJSON), &props); err != nil {
		return nil, fmt.Errorf("unmarshal properties: %w", err)
	}
	n := &graph.Node{
		ID:         graph.NodeID(id),
		Domain:     graph.DomainID(domain),
		Layer:      layer,
		Name:       name,
		Properties: props,
		Revision:   rev,
		CreatedAt:  time.UnixMilli(createdAt),
		UpdatedAt:  time.UnixMilli(updatedAt),
	}
	if parent != nil {
		p := graph.NodeID(*parent)
		n.ParentID = &p
	}
	if summary != nil {
		n.Summary = *summary
	}
	return n, nil
}

func nodeIDPtr(p *graph.NodeID) *string {
	if p == nil {
		return nil
	}
	s := string(*p)
	return &s
}
