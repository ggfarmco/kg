package testutil

import (
	"context"
	"slices"
	"sync"

	"github.com/ggfarmco/kg/internal/graph"
)

type FakeStore struct {
	mu       sync.Mutex
	domains  map[graph.DomainID]graph.Domain
	nodes    map[graph.NodeID]graph.Node
	edges    map[graph.EdgeID]graph.Edge
	nextEdge graph.EdgeID
	inTx     bool
}

func NewFakeStore() *FakeStore {
	return &FakeStore{
		domains:  map[graph.DomainID]graph.Domain{},
		nodes:    map[graph.NodeID]graph.Node{},
		edges:    map[graph.EdgeID]graph.Edge{},
		nextEdge: 1,
	}
}

func (s *FakeStore) snapshot() *FakeStore {
	cp := &FakeStore{
		domains:  make(map[graph.DomainID]graph.Domain, len(s.domains)),
		nodes:    make(map[graph.NodeID]graph.Node, len(s.nodes)),
		edges:    make(map[graph.EdgeID]graph.Edge, len(s.edges)),
		nextEdge: s.nextEdge,
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
	return cp
}

func (s *FakeStore) restore(cp *FakeStore) {
	s.domains = cp.domains
	s.nodes = cp.nodes
	s.edges = cp.edges
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
			return graph.ErrDomainNotFound
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
			return graph.ErrNodeNotFound
		}
	}
	delete(s.nodes, id)
	for k, e := range s.edges {
		if e.SourceID == id || e.TargetID == id {
			delete(s.edges, k)
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

func (s *FakeStore) CreateEdge(_ context.Context, e *graph.Edge) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.edges {
		if existing.SourceID == e.SourceID && existing.TargetID == e.TargetID && existing.Type == e.Type {
			return graph.ErrEdgeAlreadyExists
		}
	}
	e.ID = s.nextEdge
	s.nextEdge++
	s.edges[e.ID] = *e
	return nil
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

func (s *FakeStore) DeleteEdge(_ context.Context, id graph.EdgeID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.edges[id]; !ok {
		return graph.ErrEdgeNotFound
	}
	delete(s.edges, id)
	return nil
}

func (s *FakeStore) EdgesFrom(_ context.Context, sourceID graph.NodeID, types []string) ([]graph.Edge, error) {
	return s.edgesMatching(func(e graph.Edge) bool { return e.SourceID == sourceID }, types), nil
}

func (s *FakeStore) EdgesTo(_ context.Context, targetID graph.NodeID, types []string) ([]graph.Edge, error) {
	return s.edgesMatching(func(e graph.Edge) bool { return e.TargetID == targetID }, types), nil
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

var _ graph.Store = (*FakeStore)(nil)
