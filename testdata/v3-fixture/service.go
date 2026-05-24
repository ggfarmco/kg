package main

import "context"

type Service struct {
	store *Store
}

func NewService(store *Store) *Service {
	return &Service{store: store}
}

func (s *Service) GetUser(ctx context.Context, id string) (*User, error) {
	return s.store.Find(ctx, id)
}
