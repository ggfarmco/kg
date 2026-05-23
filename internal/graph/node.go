package graph

import (
	"errors"
	"regexp"
	"strings"
	"time"
)

type (
	SlugID string
	NodeID string
)

type Node struct {
	ID         NodeID         `json:"id"`
	Domain     DomainID       `json:"domain"`
	Layer      string         `json:"layer"`
	Name       string         `json:"name"`
	ParentID   *NodeID        `json:"parent_id"`
	Summary    string         `json:"summary"`
	Properties map[string]any `json:"properties"`
	Revision   int64          `json:"revision"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

type NodeFilter struct {
	Domain DomainID
	Layer  string
	Limit  int
}

var slugRE = regexp.MustCompile(`^[a-z0-9-]+(?:(?:/|::)[a-z0-9-]+)*$`)

func ParseSlug(s string) (SlugID, error) {
	if !slugRE.MatchString(s) {
		return "", ErrInvalidSlug
	}
	return SlugID(s), nil
}

func NewNodeID(d DomainID, s SlugID) NodeID {
	return NodeID(string(d) + ":" + string(s))
}

func (n NodeID) Split() (DomainID, SlugID, error) {
	parts := strings.SplitN(string(n), ":", 2)
	if len(parts) != 2 {
		return "", "", errors.New("node id missing ':'")
	}
	d, err := ParseDomainID(parts[0])
	if err != nil {
		return "", "", err
	}
	s, err := ParseSlug(parts[1])
	if err != nil {
		return "", "", err
	}
	return d, s, nil
}
