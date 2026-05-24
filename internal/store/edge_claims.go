package store

import (
	"context"
	"fmt"
	"time"

	"github.com/ggfarmco/kg/internal/graph"
)

func (s *Store) AddEdgeClaim(ctx context.Context, edgeID graph.EdgeID, source graph.SourceID, at time.Time) error {
	q := New(s.conn(ctx))
	return q.AddEdgeClaim(ctx, AddEdgeClaimParams{
		EdgeID: int64(edgeID), Source: string(source), ClaimedAt: at.UnixMilli(),
	})
}

func (s *Store) RemoveEdgeClaim(ctx context.Context, edgeID graph.EdgeID, source graph.SourceID) error {
	q := New(s.conn(ctx))
	return q.RemoveEdgeClaim(ctx, RemoveEdgeClaimParams{EdgeID: int64(edgeID), Source: string(source)})
}

func (s *Store) CountEdgeClaims(ctx context.Context, edgeID graph.EdgeID) (int, error) {
	q := New(s.conn(ctx))
	n, err := q.CountEdgeClaims(ctx, int64(edgeID))
	if err != nil {
		return 0, fmt.Errorf("sqlite: count edge claims: %w", err)
	}
	return int(n), nil
}

func (s *Store) ListEdgeClaims(ctx context.Context, edgeID graph.EdgeID) ([]graph.EdgeClaim, error) {
	q := New(s.conn(ctx))
	rows, err := q.ListEdgeClaims(ctx, int64(edgeID))
	if err != nil {
		return nil, fmt.Errorf("sqlite: list edge claims: %w", err)
	}
	out := make([]graph.EdgeClaim, 0, len(rows))
	for _, r := range rows {
		out = append(out, graph.EdgeClaim{
			EdgeID: graph.EdgeID(r.EdgeID), Source: graph.SourceID(r.Source),
			ClaimedAt: time.UnixMilli(r.ClaimedAt),
		})
	}
	return out, nil
}

func (s *Store) EdgeIDsClaimedBy(ctx context.Context, source graph.SourceID) ([]graph.EdgeID, error) {
	q := New(s.conn(ctx))
	rows, err := q.EdgeIDsClaimedBy(ctx, string(source))
	if err != nil {
		return nil, fmt.Errorf("sqlite: edges claimed by: %w", err)
	}
	out := make([]graph.EdgeID, 0, len(rows))
	for _, r := range rows {
		out = append(out, graph.EdgeID(r))
	}
	return out, nil
}
