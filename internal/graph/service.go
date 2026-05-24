package graph

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Clock func() time.Time

type Service struct {
	store Store
	now   Clock
}

func NewService(store Store, now Clock) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{store: store, now: now}
}

func (s *Service) ensureSource(ctx context.Context, source SourceID) error {
	now := s.now()
	return s.store.UpsertSource(ctx, Source{
		ID:        source,
		Trust:     100,
		FirstSeen: now,
		LastSeen:  now,
	})
}

type AddDomainInput struct {
	ID          string
	Description string
	Layers      []string
	Source      string
}

func (s *Service) AddDomain(ctx context.Context, in AddDomainInput) (*Domain, error) {
	id, err := ParseDomainID(in.ID)
	if err != nil {
		return nil, err
	}
	if in.Source == "" {
		return nil, ErrSourceRequired
	}
	source, err := ParseSourceID(in.Source)
	if err != nil {
		return nil, err
	}
	if len(in.Layers) == 0 {
		return nil, errors.New("layers must not be empty")
	}
	seen := make(map[string]struct{}, len(in.Layers))
	for i, l := range in.Layers {
		if l == "" {
			return nil, fmt.Errorf("layer %d is empty", i)
		}
		if _, dup := seen[l]; dup {
			return nil, fmt.Errorf("layer %q is duplicated", l)
		}
		seen[l] = struct{}{}
	}
	d := Domain{
		ID:          id,
		Description: in.Description,
		Layers:      append([]string(nil), in.Layers...),
		Revision:    1,
		CreatedAt:   s.now(),
	}
	if err := s.ensureSource(ctx, source); err != nil {
		return nil, err
	}
	if err := s.store.CreateDomain(ctx, d); err != nil {
		return nil, err
	}
	return &d, nil
}

func (s *Service) GetDomain(ctx context.Context, id DomainID) (*Domain, error) {
	return s.store.GetDomain(ctx, id)
}

func (s *Service) ListDomains(ctx context.Context) ([]Domain, error) {
	return s.store.ListDomains(ctx)
}

func (s *Service) DeleteDomain(ctx context.Context, id DomainID) error {
	return s.store.DeleteDomain(ctx, id)
}

type AddNodeInput struct {
	Domain     string
	Layer      string
	Name       string
	ID         string
	Parent     string
	Source     string
	Properties map[string]any
}

func deriveSlug(name string) (SlugID, error) {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.ReplaceAll(s, " ", "-")
	return ParseSlug(s)
}

func (s *Service) AddNode(ctx context.Context, in AddNodeInput) (*Node, error) {
	dID, err := ParseDomainID(in.Domain)
	if err != nil {
		return nil, err
	}
	if in.Source == "" {
		return nil, ErrSourceRequired
	}
	source, err := ParseSourceID(in.Source)
	if err != nil {
		return nil, err
	}
	d, err := s.store.GetDomain(ctx, dID)
	if err != nil {
		return nil, err
	}
	if !slicesContains(d.Layers, in.Layer) {
		return nil, ErrLayerNotInDomain
	}

	var slug SlugID
	if in.ID != "" {
		slug, err = ParseSlug(in.ID)
		if err != nil {
			return nil, ErrInvalidSlug
		}
	} else {
		slug, err = deriveSlug(in.Name)
		if err != nil {
			return nil, ErrSlugCannotDerive
		}
	}

	topLayer := d.Layers[0]
	isTop := in.Layer == topLayer

	var parentPtr *NodeID
	if in.Parent != "" {
		if isTop {
			return nil, ErrTopLayerCannotHaveParent
		}
		parentID := NodeID(in.Parent)
		parent, err := s.store.GetNode(ctx, parentID)
		if err != nil {
			return nil, err
		}
		if parent.Domain != dID {
			return nil, ErrParentDomainMismatch
		}
		parentLayerIdx := indexOf(d.Layers, parent.Layer)
		nodeLayerIdx := indexOf(d.Layers, in.Layer)
		if parentLayerIdx < 0 || nodeLayerIdx < 0 || nodeLayerIdx-parentLayerIdx != 1 {
			return nil, ErrParentLayerMismatch
		}
		parentPtr = &parentID
	} else if !isTop {
		return nil, ErrParentLayerMismatch
	}

	id := NewNodeID(dID, slug)
	existing, getErr := s.store.GetNode(ctx, id)
	if getErr != nil && !errors.Is(getErr, ErrNodeNotFound) {
		return nil, getErr
	}
	if existing != nil {
		if existing.Source != source {
			return nil, ErrNodeOwnedByDifferentSource
		}
		return nil, ErrNodeAlreadyExists
	}

	if err := s.ensureSource(ctx, source); err != nil {
		return nil, err
	}
	now := s.now()
	n := Node{
		ID:         id,
		Domain:     dID,
		Layer:      in.Layer,
		Name:       in.Name,
		ParentID:   parentPtr,
		Source:     source,
		Properties: nonNilNamespacedProps(source, in.Properties),
		Revision:   1,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := s.store.CreateNode(ctx, n); err != nil {
		return nil, err
	}
	return &n, nil
}

func nonNilNamespacedProps(source SourceID, props map[string]any) map[SourceID]map[string]any {
	out := map[SourceID]map[string]any{}
	if len(props) == 0 {
		return out
	}
	cp := make(map[string]any, len(props))
	for k, v := range props {
		cp[k] = v
	}
	out[source] = cp
	return out
}

type UpdateNodeInput struct {
	Source SourceID
	Name   *string
}

func (s *Service) GetNode(ctx context.Context, id NodeID) (*Node, error) {
	return s.store.GetNode(ctx, id)
}

func (s *Service) ListNodes(ctx context.Context, f NodeFilter) ([]Node, error) {
	return s.store.ListNodes(ctx, f)
}

func (s *Service) ChildrenOf(ctx context.Context, id NodeID) ([]Node, error) {
	return s.store.ChildrenOf(ctx, id)
}

func (s *Service) UpdateNode(ctx context.Context, id NodeID, in UpdateNodeInput) (*Node, error) {
	if in.Source == "" {
		return nil, ErrSourceRequired
	}
	cur, err := s.store.GetNode(ctx, id)
	if err != nil {
		return nil, err
	}
	if cur.Source != in.Source {
		return nil, ErrNodeNotOwner
	}
	if in.Name != nil {
		cur.Name = *in.Name
	}
	cur.UpdatedAt = s.now()
	if err := s.store.UpdateNode(ctx, *cur); err != nil {
		return nil, err
	}
	return s.store.GetNode(ctx, id)
}

func (s *Service) DeleteNode(ctx context.Context, id NodeID, source SourceID) error {
	if source == "" {
		return ErrSourceRequired
	}
	cur, err := s.store.GetNode(ctx, id)
	if err != nil {
		return err
	}
	if cur.Source != source {
		return ErrNodeNotOwner
	}
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
			if c.Source != source {
				return ErrNodeHasForeignClaims
			}
		}
	}
	return s.store.DeleteNode(ctx, id)
}

type AddEdgeInput struct {
	Source       string
	Target       string
	Type         string
	Properties   map[string]any
	WriterSource string
}

func (s *Service) AddEdge(ctx context.Context, in AddEdgeInput) (*Edge, error) {
	src := NodeID(in.Source)
	dst := NodeID(in.Target)
	if src == dst {
		return nil, ErrEdgeSelfLoop
	}
	if in.Type == "" {
		return nil, fmt.Errorf("edge type must not be empty")
	}
	if in.WriterSource == "" {
		return nil, ErrSourceRequired
	}
	source, err := ParseSourceID(in.WriterSource)
	if err != nil {
		return nil, err
	}
	if _, err := s.store.GetNode(ctx, src); err != nil {
		return nil, err
	}
	if _, err := s.store.GetNode(ctx, dst); err != nil {
		return nil, err
	}
	if err := s.ensureSource(ctx, source); err != nil {
		return nil, err
	}
	now := s.now()
	id, err := s.store.UpsertEdge(ctx, Edge{
		SourceID:   src,
		TargetID:   dst,
		Type:       in.Type,
		Properties: nonNilNamespacedProps(source, in.Properties),
		CreatedAt:  now,
	})
	if err != nil {
		return nil, err
	}
	if err := s.store.AddEdgeClaim(ctx, id, source, now); err != nil {
		return nil, err
	}
	got, err := s.store.GetEdge(ctx, id)
	if err != nil {
		return nil, err
	}
	claims, err := s.store.ListEdgeClaims(ctx, id)
	if err != nil {
		return nil, err
	}
	got.Claims = make([]SourceID, 0, len(claims))
	for _, c := range claims {
		got.Claims = append(got.Claims, c.Source)
	}
	return got, nil
}

func (s *Service) AddEdgeClaim(ctx context.Context, id EdgeID, source SourceID) error {
	if source == "" {
		return ErrSourceRequired
	}
	if err := s.ensureSource(ctx, source); err != nil {
		return err
	}
	return s.store.AddEdgeClaim(ctx, id, source, s.now())
}

func (s *Service) RemoveEdgeClaim(ctx context.Context, id EdgeID, source SourceID) error {
	if source == "" {
		return ErrSourceRequired
	}
	return s.store.InTx(ctx, func(ctx context.Context) error {
		if err := s.store.RemoveEdgeClaim(ctx, id, source); err != nil {
			return err
		}
		n, err := s.store.CountEdgeClaims(ctx, id)
		if err != nil {
			return err
		}
		if n == 0 {
			return s.store.DeleteEdge(ctx, id)
		}
		return nil
	})
}

func (s *Service) ListEdgeClaims(ctx context.Context, id EdgeID) ([]EdgeClaim, error) {
	return s.store.ListEdgeClaims(ctx, id)
}

func (s *Service) DeleteEdge(ctx context.Context, id EdgeID) error {
	return s.store.DeleteEdge(ctx, id)
}

func (s *Service) EdgesFrom(ctx context.Context, src NodeID, types []string) ([]Edge, error) {
	return s.store.EdgesFrom(ctx, src, types)
}

func (s *Service) EdgesTo(ctx context.Context, dst NodeID, types []string) ([]Edge, error) {
	return s.store.EdgesTo(ctx, dst, types)
}

func (s *Service) InTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return s.store.InTx(ctx, fn)
}

func slicesContains[T comparable](haystack []T, needle T) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

func indexOf[T comparable](xs []T, target T) int {
	for i, x := range xs {
		if x == target {
			return i
		}
	}
	return -1
}

func nonNilProps(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (s *Service) SetNodeProperties(ctx context.Context, id NodeID, source SourceID, props map[string]any) error {
	if source == "" {
		return ErrSourceRequired
	}
	if err := s.ensureSource(ctx, source); err != nil {
		return err
	}
	cur, err := s.store.GetNode(ctx, id)
	if err != nil {
		return err
	}
	if cur.Properties == nil {
		cur.Properties = map[SourceID]map[string]any{}
	}
	copy := make(map[string]any, len(props))
	for k, v := range props {
		copy[k] = v
	}
	cur.Properties[source] = copy
	cur.UpdatedAt = s.now()
	return s.store.UpdateNode(ctx, *cur)
}

func (s *Service) DeleteNodeProperties(ctx context.Context, id NodeID, source SourceID) error {
	if source == "" {
		return ErrSourceRequired
	}
	cur, err := s.store.GetNode(ctx, id)
	if err != nil {
		return err
	}
	if cur.Properties != nil {
		delete(cur.Properties, source)
	}
	cur.UpdatedAt = s.now()
	return s.store.UpdateNode(ctx, *cur)
}

func (s *Service) SetEdgeProperties(ctx context.Context, id EdgeID, source SourceID, props map[string]any) error {
	if source == "" {
		return ErrSourceRequired
	}
	if err := s.ensureSource(ctx, source); err != nil {
		return err
	}
	cur, err := s.store.GetEdge(ctx, id)
	if err != nil {
		return err
	}
	if cur.Properties == nil {
		cur.Properties = map[SourceID]map[string]any{}
	}
	copy := make(map[string]any, len(props))
	for k, v := range props {
		copy[k] = v
	}
	cur.Properties[source] = copy
	return s.store.UpdateEdgeProperties(ctx, id, cur.Properties)
}
