package graph

import (
	"context"
	"errors"
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
