package testutil

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/ggfarmco/kg/internal/graph"
)

type FakeStore struct {
	mu       sync.Mutex
	sources  map[graph.SourceID]graph.Source
	domains  map[graph.DomainID]graph.Domain
	nodes    map[graph.NodeID]graph.Node
	edges    map[graph.EdgeID]graph.Edge
	claims   map[graph.EdgeID]map[graph.SourceID]graph.EdgeClaim
	nextEdge graph.EdgeID
	inTx     bool
}

func NewFakeStore() *FakeStore {
	return &FakeStore{
		sources:  map[graph.SourceID]graph.Source{},
		domains:  map[graph.DomainID]graph.Domain{},
		nodes:    map[graph.NodeID]graph.Node{},
		edges:    map[graph.EdgeID]graph.Edge{},
		claims:   map[graph.EdgeID]map[graph.SourceID]graph.EdgeClaim{},
		nextEdge: 1,
	}
}

func (s *FakeStore) snapshot() *FakeStore {
	cp := &FakeStore{
		sources:  make(map[graph.SourceID]graph.Source, len(s.sources)),
		domains:  make(map[graph.DomainID]graph.Domain, len(s.domains)),
		nodes:    make(map[graph.NodeID]graph.Node, len(s.nodes)),
		edges:    make(map[graph.EdgeID]graph.Edge, len(s.edges)),
		claims:   make(map[graph.EdgeID]map[graph.SourceID]graph.EdgeClaim, len(s.claims)),
		nextEdge: s.nextEdge,
	}
	for k, v := range s.sources {
		cp.sources[k] = v
	}
	for k, v := range s.domains {
		cp.domains[k] = v
	}
	for k, v := range s.nodes {
		cp.nodes[k] = v
	}
	for k, v := range s.edges {
		cp.edges[k] = v
	}
	for k, inner := range s.claims {
		ic := make(map[graph.SourceID]graph.EdgeClaim, len(inner))
		for sk, sv := range inner {
			ic[sk] = sv
		}
		cp.claims[k] = ic
	}
	return cp
}

func (s *FakeStore) restore(cp *FakeStore) {
	s.sources = cp.sources
	s.domains = cp.domains
	s.nodes = cp.nodes
	s.edges = cp.edges
	s.claims = cp.claims
	s.nextEdge = cp.nextEdge
}

func (s *FakeStore) InTx(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	s.mu.Lock()
	if s.inTx {
		s.mu.Unlock()
		return graph.ErrNestedTransaction
	}
	s.inTx = true
	cp := s.snapshot()
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if r := recover(); r != nil {
			s.restore(cp)
			s.inTx = false
			panic(r)
		}
		if err != nil {
			s.restore(cp)
		}
		s.inTx = false
	}()

	return fn(ctx)
}

func (s *FakeStore) InTxOrConn(ctx context.Context, fn func(ctx context.Context) error) error {
	s.mu.Lock()
	already := s.inTx
	s.mu.Unlock()
	if already {
		return fn(ctx)
	}
	return s.InTx(ctx, fn)
}

func (s *FakeStore) UpsertSource(_ context.Context, src graph.Source) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cur, ok := s.sources[src.ID]; ok {
		cur.LastSeen = src.LastSeen
		s.sources[src.ID] = cur
		return nil
	}
	s.sources[src.ID] = src
	return nil
}

func (s *FakeStore) GetSource(_ context.Context, id graph.SourceID) (*graph.Source, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.sources[id]
	if !ok {
		return nil, graph.ErrSourceNotFound
	}
	return &v, nil
}

func (s *FakeStore) ListSources(_ context.Context) ([]graph.Source, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]graph.Source, 0, len(s.sources))
	for _, v := range s.sources {
		out = append(out, v)
	}
	slices.SortFunc(out, func(a, b graph.Source) int {
		switch {
		case a.ID < b.ID:
			return -1
		case a.ID > b.ID:
			return 1
		default:
			return 0
		}
	})
	return out, nil
}

func (s *FakeStore) UpdateSource(_ context.Context, src graph.Source) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.sources[src.ID]
	if !ok {
		return graph.ErrSourceNotFound
	}
	cur.Description = src.Description
	s.sources[src.ID] = cur
	return nil
}

func (s *FakeStore) DeleteSource(_ context.Context, id graph.SourceID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sources[id]; !ok {
		return graph.ErrSourceNotFound
	}
	for _, n := range s.nodes {
		if n.Source == id {
			return graph.ErrSourceHasDependents
		}
	}
	for _, ic := range s.claims {
		if _, ok := ic[id]; ok {
			return graph.ErrSourceHasDependents
		}
	}
	delete(s.sources, id)
	return nil
}

func (s *FakeStore) CreateDomain(_ context.Context, d graph.Domain) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.domains[d.ID]; ok {
		return graph.ErrDomainAlreadyExists
	}
	s.domains[d.ID] = d
	return nil
}

func (s *FakeStore) GetDomain(_ context.Context, id graph.DomainID) (*graph.Domain, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.domains[id]
	if !ok {
		return nil, graph.ErrDomainNotFound
	}
	return &d, nil
}

func (s *FakeStore) ListDomains(_ context.Context) ([]graph.Domain, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]graph.Domain, 0, len(s.domains))
	for _, d := range s.domains {
		out = append(out, d)
	}
	slices.SortFunc(out, func(a, b graph.Domain) int {
		switch {
		case a.ID < b.ID:
			return -1
		case a.ID > b.ID:
			return 1
		default:
			return 0
		}
	})
	return out, nil
}

func (s *FakeStore) DeleteDomain(_ context.Context, id graph.DomainID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.domains[id]; !ok {
		return graph.ErrDomainNotFound
	}
	for _, n := range s.nodes {
		if n.Domain == id {
			return graph.ErrHasDependents
		}
	}
	delete(s.domains, id)
	return nil
}

func (s *FakeStore) CreateNode(_ context.Context, n graph.Node) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.nodes[n.ID]; ok {
		return graph.ErrNodeAlreadyExists
	}
	if _, ok := s.sources[n.Source]; !ok {
		return graph.ErrSourceNotFound
	}
	s.nodes[n.ID] = n
	return nil
}

func (s *FakeStore) GetNode(_ context.Context, id graph.NodeID) (*graph.Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, ok := s.nodes[id]
	if !ok {
		return nil, graph.ErrNodeNotFound
	}
	return &n, nil
}

func (s *FakeStore) ListNodes(_ context.Context, f graph.NodeFilter) ([]graph.Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]graph.Node, 0, len(s.nodes))
	for _, n := range s.nodes {
		if f.Domain != "" && n.Domain != f.Domain {
			continue
		}
		if f.Layer != "" && n.Layer != f.Layer {
			continue
		}
		if f.Source != "" && n.Source != f.Source {
			continue
		}
		out = append(out, n)
	}
	slices.SortFunc(out, func(a, b graph.Node) int {
		switch {
		case a.ID < b.ID:
			return -1
		case a.ID > b.ID:
			return 1
		default:
			return 0
		}
	})
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out, nil
}

func (s *FakeStore) UpdateNode(_ context.Context, n graph.Node) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.nodes[n.ID]
	if !ok {
		return graph.ErrNodeNotFound
	}
	n.Revision = cur.Revision + 1
	s.nodes[n.ID] = n
	return nil
}

func (s *FakeStore) DeleteNode(_ context.Context, id graph.NodeID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.nodes[id]; !ok {
		return graph.ErrNodeNotFound
	}
	for _, child := range s.nodes {
		if child.ParentID != nil && *child.ParentID == id {
			return graph.ErrHasDependents
		}
	}
	delete(s.nodes, id)
	for k, e := range s.edges {
		if e.SourceID == id || e.TargetID == id {
			delete(s.edges, k)
			delete(s.claims, k)
		}
	}
	return nil
}

func (s *FakeStore) ChildrenOf(_ context.Context, parentID graph.NodeID) ([]graph.Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]graph.Node, 0)
	for _, n := range s.nodes {
		if n.ParentID != nil && *n.ParentID == parentID {
			out = append(out, n)
		}
	}
	slices.SortFunc(out, func(a, b graph.Node) int {
		switch {
		case a.ID < b.ID:
			return -1
		case a.ID > b.ID:
			return 1
		default:
			return 0
		}
	})
	return out, nil
}

func (s *FakeStore) NodesOwnedBy(_ context.Context, domain graph.DomainID, source graph.SourceID) ([]graph.Node, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]graph.Node, 0)
	for _, n := range s.nodes {
		if n.Domain == domain && n.Source == source {
			out = append(out, n)
		}
	}
	slices.SortFunc(out, func(a, b graph.Node) int {
		switch {
		case a.ID < b.ID:
			return -1
		case a.ID > b.ID:
			return 1
		default:
			return 0
		}
	})
	return out, nil
}

func (s *FakeStore) UpsertEdge(_ context.Context, e graph.Edge) (graph.EdgeID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, existing := range s.edges {
		if existing.SourceID == e.SourceID && existing.TargetID == e.TargetID && existing.Type == e.Type {
			return id, nil
		}
	}
	id := s.nextEdge
	s.nextEdge++
	e.ID = id
	if e.Properties == nil {
		e.Properties = map[graph.SourceID]map[string]any{}
	}
	s.edges[id] = e
	return id, nil
}

func (s *FakeStore) GetEdge(_ context.Context, id graph.EdgeID) (*graph.Edge, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.edges[id]
	if !ok {
		return nil, graph.ErrEdgeNotFound
	}
	return &e, nil
}

func (s *FakeStore) UpdateEdgeProperties(_ context.Context, id graph.EdgeID, props map[graph.SourceID]map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.edges[id]
	if !ok {
		return graph.ErrEdgeNotFound
	}
	e.Properties = props
	e.Revision++
	s.edges[id] = e
	return nil
}

func (s *FakeStore) DeleteEdge(_ context.Context, id graph.EdgeID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.edges[id]; !ok {
		return graph.ErrEdgeNotFound
	}
	delete(s.edges, id)
	delete(s.claims, id)
	return nil
}

func (s *FakeStore) EdgesFrom(_ context.Context, src graph.NodeID, types []string) ([]graph.Edge, error) {
	return s.edgesMatching(func(e graph.Edge) bool { return e.SourceID == src }, types), nil
}

func (s *FakeStore) EdgesTo(_ context.Context, dst graph.NodeID, types []string) ([]graph.Edge, error) {
	return s.edgesMatching(func(e graph.Edge) bool { return e.TargetID == dst }, types), nil
}

func (s *FakeStore) edgesMatching(pred func(graph.Edge) bool, types []string) []graph.Edge {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]graph.Edge, 0)
	for _, e := range s.edges {
		if !pred(e) {
			continue
		}
		if len(types) > 0 && !slices.Contains(types, e.Type) {
			continue
		}
		out = append(out, e)
	}
	slices.SortFunc(out, func(a, b graph.Edge) int {
		switch {
		case a.ID < b.ID:
			return -1
		case a.ID > b.ID:
			return 1
		default:
			return 0
		}
	})
	return out
}

func (s *FakeStore) AddEdgeClaim(_ context.Context, id graph.EdgeID, source graph.SourceID, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.edges[id]; !ok {
		return graph.ErrEdgeNotFound
	}
	if _, ok := s.sources[source]; !ok {
		return graph.ErrSourceNotFound
	}
	inner, ok := s.claims[id]
	if !ok {
		inner = map[graph.SourceID]graph.EdgeClaim{}
		s.claims[id] = inner
	}
	if _, exists := inner[source]; exists {
		return nil
	}
	inner[source] = graph.EdgeClaim{EdgeID: id, Source: source, ClaimedAt: at}
	return nil
}

func (s *FakeStore) RemoveEdgeClaim(_ context.Context, id graph.EdgeID, source graph.SourceID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	inner, ok := s.claims[id]
	if !ok {
		return nil
	}
	delete(inner, source)
	if len(inner) == 0 {
		delete(s.claims, id)
	}
	return nil
}

func (s *FakeStore) CountEdgeClaims(_ context.Context, id graph.EdgeID) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.claims[id]), nil
}

func (s *FakeStore) ListEdgeClaims(_ context.Context, id graph.EdgeID) ([]graph.EdgeClaim, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	inner := s.claims[id]
	out := make([]graph.EdgeClaim, 0, len(inner))
	for _, c := range inner {
		out = append(out, c)
	}
	slices.SortFunc(out, func(a, b graph.EdgeClaim) int {
		switch {
		case a.Source < b.Source:
			return -1
		case a.Source > b.Source:
			return 1
		default:
			return 0
		}
	})
	return out, nil
}

func (s *FakeStore) EdgeIDsClaimedBy(_ context.Context, source graph.SourceID) ([]graph.EdgeID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]graph.EdgeID, 0)
	for id, inner := range s.claims {
		if _, ok := inner[source]; ok {
			out = append(out, id)
		}
	}
	slices.SortFunc(out, func(a, b graph.EdgeID) int {
		switch {
		case a < b:
			return -1
		case a > b:
			return 1
		default:
			return 0
		}
	})
	return out, nil
}

var _ graph.Store = (*FakeStore)(nil)
