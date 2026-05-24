package graph

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ggfarmco/kg/snapshot"
)

type ApplyOptions struct {
	OverrideScope       snapshot.Scope
	DryRun              bool
	ForceCascade        bool
	ForceDomainTakeover bool
}

type ApplyResult struct {
	Source        string `json:"source"`
	Domain        string `json:"domain"`
	Scope         string `json:"scope"`
	NodesAdded    int    `json:"nodes_added"`
	NodesUpdated  int    `json:"nodes_updated"`
	NodesRemoved  int    `json:"nodes_removed"`
	EdgesAdded    int    `json:"edges_added"`
	ClaimsAdded   int    `json:"claims_added"`
	ClaimsRemoved int    `json:"claims_removed"`
	EdgesGC       int    `json:"edges_gc"`
	TookMs        int    `json:"took_ms"`
	DryRun        bool   `json:"dry_run,omitempty"`
}

var errApplyDryRun = errors.New("apply: dry-run rollback")

func (s *Service) Apply(ctx context.Context, snap snapshot.Snapshot, opts ApplyOptions) (*ApplyResult, error) {
	if err := snapshot.Validate(&snap); err != nil {
		return nil, err
	}
	source, err := ParseSourceID(snap.Source)
	if err != nil {
		return nil, err
	}
	domainID, err := ParseDomainID(snap.Domain)
	if err != nil {
		return nil, err
	}
	scope := snap.Scope
	if opts.OverrideScope != "" {
		scope = opts.OverrideScope
	}

	start := time.Now()
	res := &ApplyResult{
		Source: snap.Source, Domain: snap.Domain, Scope: string(scope),
	}

	apply := func(ctx context.Context) error {
		if err := s.ensureSource(ctx, source); err != nil {
			return err
		}
		if scope == snapshot.ScopeDomain && !opts.ForceDomainTakeover {
			rows, err := s.store.ListNodes(ctx, NodeFilter{Domain: domainID})
			if err != nil {
				return err
			}
			for _, n := range rows {
				if n.Source != source {
					return fmt.Errorf("%w: domain=%s foreign=%s", ErrDomainHasForeignWriters, domainID, n.Source)
				}
			}
		}
		if err := s.applyDomainSpec(ctx, snap.DomainSpec, source); err != nil {
			return err
		}
		sorted, err := snapshot.TopoSortNodes(snap.Nodes)
		if err != nil {
			return err
		}
		existingNodes, err := s.store.NodesOwnedBy(ctx, domainID, source)
		if err != nil {
			return err
		}
		byID := make(map[NodeID]Node, len(existingNodes))
		for _, n := range existingNodes {
			byID[n.ID] = n
		}
		for _, spec := range sorted {
			if err := s.applyNodeSpec(ctx, spec, source, byID, res, scope); err != nil {
				return err
			}
		}
		if err := s.applyEdges(ctx, snap, source, res); err != nil {
			return err
		}
		if scope != snapshot.ScopeAdditive {
			if err := s.applyCleanup(ctx, byID, source, opts, res); err != nil {
				return err
			}
		}
		return nil
	}

	if opts.DryRun {
		txErr := s.store.InTx(ctx, func(ctx context.Context) error {
			if err := apply(ctx); err != nil {
				return err
			}
			return errApplyDryRun
		})
		if !errors.Is(txErr, errApplyDryRun) {
			return nil, txErr
		}
		res.DryRun = true
		res.TookMs = int(time.Since(start).Milliseconds())
		return res, nil
	}

	if err := s.store.InTx(ctx, apply); err != nil {
		return nil, err
	}
	res.TookMs = int(time.Since(start).Milliseconds())
	return res, nil
}

func (s *Service) applyDomainSpec(ctx context.Context, spec *snapshot.DomainSpec, source SourceID) error {
	if spec == nil {
		return nil
	}
	existing, err := s.store.GetDomain(ctx, DomainID(spec.ID))
	if err != nil && !errors.Is(err, ErrDomainNotFound) {
		return err
	}
	if existing == nil {
		_, err := s.AddDomain(ctx, AddDomainInput{
			ID: spec.ID, Description: spec.Description,
			Layers: spec.Layers, Source: string(source),
		})
		return err
	}
	return nil
}

func (s *Service) applyNodeSpec(
	ctx context.Context, spec snapshot.NodeSpec, source SourceID,
	byID map[NodeID]Node, res *ApplyResult, scope snapshot.Scope,
) error {
	id := NodeID(spec.ID)
	existing, ok := byID[id]
	if !ok {
		other, gerr := s.store.GetNode(ctx, id)
		if gerr == nil && other != nil {
			if scope == snapshot.ScopeAdditive {
				return nil
			}
			return fmt.Errorf("%w: id=%s owner=%s", ErrNodeOwnedByDifferentSource, id, other.Source)
		}
		if gerr != nil && !errors.Is(gerr, ErrNodeNotFound) {
			return gerr
		}
		_, err := s.AddNode(ctx, AddNodeInput{
			Domain: string(domainFromID(id)), Layer: spec.Layer, Name: spec.Name,
			ID: string(slugFromID(id)), Parent: spec.Parent,
			Source: string(source), Properties: spec.Properties,
		})
		if err != nil {
			return err
		}
		res.NodesAdded++
		return nil
	}
	if existing.Layer != spec.Layer || parentString(existing.ParentID) != spec.Parent {
		return fmt.Errorf("%w: id=%s", ErrCoreFieldsImmutable, id)
	}
	changed := false
	if existing.Name != spec.Name {
		existing.Name = spec.Name
		changed = true
	}
	newProps := nonNilNamespacedProps(source, spec.Properties)
	if !propsEqual(existing.Properties[source], newProps[source]) {
		if existing.Properties == nil {
			existing.Properties = map[SourceID]map[string]any{}
		}
		existing.Properties[source] = newProps[source]
		changed = true
	}
	if changed {
		existing.UpdatedAt = s.now()
		if err := s.store.UpdateNode(ctx, existing); err != nil {
			return err
		}
		res.NodesUpdated++
	}
	delete(byID, id)
	return nil
}

func (s *Service) applyCleanup(
	ctx context.Context, residual map[NodeID]Node, source SourceID, opts ApplyOptions, res *ApplyResult,
) error {
	for id := range residual {
		incoming, err := s.store.EdgesTo(ctx, id, nil)
		if err != nil {
			return err
		}
		outgoing, err := s.store.EdgesFrom(ctx, id, nil)
		if err != nil {
			return err
		}
		for _, e := range append(incoming, outgoing...) {
			claims, err := s.store.ListEdgeClaims(ctx, e.ID)
			if err != nil {
				return err
			}
			for _, c := range claims {
				if c.Source == source {
					continue
				}
				if !opts.ForceCascade {
					return fmt.Errorf("%w: node=%s edge=%d", ErrNodeHasForeignClaims, id, e.ID)
				}
			}
		}
		children, err := s.store.ChildrenOf(ctx, id)
		if err != nil {
			return err
		}
		if len(children) > 0 && !opts.ForceCascade {
			return fmt.Errorf("%w: node=%s children=%d", ErrHasDependents, id, len(children))
		}
		if err := s.store.DeleteNode(ctx, id); err != nil {
			return err
		}
		res.NodesRemoved++
	}
	return nil
}

func (s *Service) applyEdges(ctx context.Context, snap snapshot.Snapshot, source SourceID, res *ApplyResult) error {
	prevClaimedIDs, err := s.store.EdgeIDsClaimedBy(ctx, source)
	if err != nil {
		return err
	}
	prevClaimed := make(map[EdgeID]struct{}, len(prevClaimedIDs))
	for _, id := range prevClaimedIDs {
		prevClaimed[id] = struct{}{}
	}

	now := s.now()
	for _, e := range snap.Edges {
		src := NodeID(e.Src)
		dst := NodeID(e.Target)
		if _, err := s.store.GetNode(ctx, src); err != nil {
			return fmt.Errorf("edges[]: source: %w", err)
		}
		if _, err := s.store.GetNode(ctx, dst); err != nil {
			return fmt.Errorf("edges[]: target: %w", err)
		}
		id, err := s.store.UpsertEdge(ctx, Edge{
			SourceID:   src,
			TargetID:   dst,
			Type:       e.Type,
			Properties: nonNilNamespacedProps(source, e.Properties),
			CreatedAt:  now,
		})
		if err != nil {
			return err
		}
		if _, already := prevClaimed[id]; !already {
			res.EdgesAdded++
		}
		if err := s.store.AddEdgeClaim(ctx, id, source, now); err != nil {
			return err
		}
		if _, already := prevClaimed[id]; !already {
			res.ClaimsAdded++
		}
		delete(prevClaimed, id)
	}

	if snap.Scope != snapshot.ScopeAdditive {
		for id := range prevClaimed {
			if err := s.store.RemoveEdgeClaim(ctx, id, source); err != nil {
				return err
			}
			res.ClaimsRemoved++
			n, err := s.store.CountEdgeClaims(ctx, id)
			if err != nil {
				return err
			}
			if n == 0 {
				if err := s.store.DeleteEdge(ctx, id); err != nil {
					return err
				}
				res.EdgesGC++
			}
		}
	}
	return nil
}

func domainFromID(id NodeID) DomainID {
	dom, _, _ := id.Split()
	return dom
}

func slugFromID(id NodeID) SlugID {
	_, slug, _ := id.Split()
	return slug
}

func parentString(p *NodeID) string {
	if p == nil {
		return ""
	}
	return string(*p)
}

func propsEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, va := range a {
		vb, ok := b[k]
		if !ok || fmt.Sprintf("%v", va) != fmt.Sprintf("%v", vb) {
			return false
		}
	}
	return true
}
