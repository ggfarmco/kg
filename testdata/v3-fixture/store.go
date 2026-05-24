package main

import "context"

type User struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Store struct {
	rows map[string]*User
}

func NewStore() *Store {
	return &Store{rows: map[string]*User{
		"1": {ID: "1", Name: "Alice"},
	}}
}

func (s *Store) Find(_ context.Context, id string) (*User, error) {
	return s.rows[id], nil
}
