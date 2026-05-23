package graph

import (
	"context"
	"errors"
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

type AddDomainInput struct {
	ID          string
	Description string
	Layers      []string
}

func (s *Service) AddDomain(ctx context.Context, in AddDomainInput) (*Domain, error) {
	id, err := ParseDomainID(in.ID)
	if err != nil {
		return nil, err
	}
	if len(in.Layers) == 0 {
		return nil, errors.New("layers must not be empty")
	}
	d := Domain{
		ID:          id,
		Description: in.Description,
		Layers:      append([]string(nil), in.Layers...),
		Revision:    1,
		CreatedAt:   s.now(),
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
	Domain  string
	Layer   string
	Name    string
	ID      string
	Parent  string
	Summary string
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

	now := s.now()
	n := Node{
		ID:         NewNodeID(dID, slug),
		Domain:     dID,
		Layer:      in.Layer,
		Name:       in.Name,
		ParentID:   parentPtr,
		Summary:    in.Summary,
		Properties: map[string]any{},
		Revision:   1,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := s.store.CreateNode(ctx, n); err != nil {
		return nil, err
	}
	return &n, nil
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
