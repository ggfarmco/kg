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
	props, err := encodeNamespacedProps(n.Properties)
	if err != nil {
		return err
	}
	return s.inTxOrConn(ctx, func(ctx context.Context) error {
		q := New(s.conn(ctx))
		if err := q.CreateNode(ctx, CreateNodeParams{
			ID:         string(n.ID),
			Domain:     string(n.Domain),
			Layer:      n.Layer,
			Name:       n.Name,
			ParentID:   nodeIDPtr(n.ParentID),
			Source:     string(n.Source),
			Properties: props,
			CreatedAt:  n.CreatedAt.UnixMilli(),
			UpdatedAt:  n.UpdatedAt.UnixMilli(),
		}); err != nil {
			return mapSQLiteErr(err, "node")
		}
		rev := int64(1)
		src := string(n.Source)
		return q.AppendChange(ctx, AppendChangeParams{
			Entity:   "node",
			EntityID: string(n.ID),
			Source:   &src,
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
	return decodeNode(row.ID, row.Domain, row.Layer, row.Name, row.ParentID, row.Source, row.Properties, row.Revision, row.CreatedAt, row.UpdatedAt)
}

func (s *Store) ListNodes(ctx context.Context, f graph.NodeFilter) ([]graph.Node, error) {
	q := New(s.conn(ctx))
	rows, err := q.ListNodes(ctx, ListNodesParams{
		DomainFilter: string(f.Domain),
		LayerFilter:  f.Layer,
		SourceFilter: string(f.Source),
		Lim:          int64(f.Limit),
	})
	if err != nil {
		return nil, fmt.Errorf("sqlite: list nodes: %w", err)
	}
	out := make([]graph.Node, 0, len(rows))
	for _, r := range rows {
		n, err := decodeNode(r.ID, r.Domain, r.Layer, r.Name, r.ParentID, r.Source, r.Properties, r.Revision, r.CreatedAt, r.UpdatedAt)
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
		n, err := decodeNode(r.ID, r.Domain, r.Layer, r.Name, r.ParentID, r.Source, r.Properties, r.Revision, r.CreatedAt, r.UpdatedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, *n)
	}
	return out, nil
}

func (s *Store) UpdateNode(ctx context.Context, n graph.Node) error {
	props, err := encodeNamespacedProps(n.Properties)
	if err != nil {
		return err
	}
	return s.inTxOrConn(ctx, func(ctx context.Context) error {
		q := New(s.conn(ctx))
		existing, err := q.GetNode(ctx, string(n.ID))
		if err != nil {
			return mapSQLiteErr(err, "node")
		}
		if err := q.UpdateNode(ctx, UpdateNodeParams{
			ID:         string(n.ID),
			Name:       n.Name,
			Properties: props,
			UpdatedAt:  n.UpdatedAt.UnixMilli(),
		}); err != nil {
			return mapSQLiteErr(err, "node")
		}
		rev, err := q.GetNodeRevision(ctx, string(n.ID))
		if err != nil {
			return fmt.Errorf("sqlite: get node revision: %w", err)
		}
		ownerSrc := existing.Source
		return q.AppendChange(ctx, AppendChangeParams{
			Entity:   "node",
			EntityID: string(n.ID),
			Source:   &ownerSrc,
			Op:       "update",
			Revision: &rev,
			At:       n.UpdatedAt.UnixMilli(),
		})
	})
}

func (s *Store) DeleteNode(ctx context.Context, id graph.NodeID) error {
	return s.inTxOrConn(ctx, func(ctx context.Context) error {
		q := New(s.conn(ctx))
		existing, err := q.GetNode(ctx, string(id))
		if err != nil {
			return mapSQLiteErr(err, "node")
		}
		if err := q.DeleteNode(ctx, string(id)); err != nil {
			if isFKViolation(err) {
				return graph.ErrHasDependents
			}
			return mapSQLiteErr(err, "node")
		}
		ownerSrc := existing.Source
		return q.AppendChange(ctx, AppendChangeParams{
			Entity:   "node",
			EntityID: string(id),
			Source:   &ownerSrc,
			Op:       "delete",
			Revision: nil,
			At:       time.Now().UnixMilli(),
		})
	})
}

func (s *Store) NodesOwnedBy(ctx context.Context, domain graph.DomainID, source graph.SourceID) ([]graph.Node, error) {
	q := New(s.conn(ctx))
	rows, err := q.NodesOwnedBy(ctx, NodesOwnedByParams{Domain: string(domain), Source: string(source)})
	if err != nil {
		return nil, fmt.Errorf("sqlite: nodes owned by: %w", err)
	}
	out := make([]graph.Node, 0, len(rows))
	for _, r := range rows {
		n, err := decodeNode(r.ID, r.Domain, r.Layer, r.Name, r.ParentID, r.Source, r.Properties, r.Revision, r.CreatedAt, r.UpdatedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, *n)
	}
	return out, nil
}

func decodeNode(id, domain, layer, name string, parent *string, source, propsJSON string, rev, createdAt, updatedAt int64) (*graph.Node, error) {
	props, err := decodeNamespacedProps(propsJSON)
	if err != nil {
		return nil, err
	}
	n := &graph.Node{
		ID: graph.NodeID(id), Domain: graph.DomainID(domain),
		Layer: layer, Name: name,
		Source: graph.SourceID(source), Properties: props,
		Revision:  rev,
		CreatedAt: time.UnixMilli(createdAt),
		UpdatedAt: time.UnixMilli(updatedAt),
	}
	if parent != nil {
		p := graph.NodeID(*parent)
		n.ParentID = &p
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

func encodeNamespacedProps(p map[graph.SourceID]map[string]any) (string, error) {
	if p == nil {
		return "{}", nil
	}
	conv := make(map[string]map[string]any, len(p))
	for k, v := range p {
		conv[string(k)] = v
	}
	b, err := json.Marshal(conv)
	if err != nil {
		return "", fmt.Errorf("marshal namespaced properties: %w", err)
	}
	return string(b), nil
}

func decodeNamespacedProps(s string) (map[graph.SourceID]map[string]any, error) {
	if s == "" || s == "{}" {
		return map[graph.SourceID]map[string]any{}, nil
	}
	var raw map[string]map[string]any
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		return nil, fmt.Errorf("unmarshal namespaced properties: %w", err)
	}
	out := make(map[graph.SourceID]map[string]any, len(raw))
	for k, v := range raw {
		out[graph.SourceID(k)] = v
	}
	return out, nil
}
