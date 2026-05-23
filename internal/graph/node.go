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
	ID         NodeID
	Domain     DomainID
	Layer      string
	Name       string
	ParentID   *NodeID
	Summary    string
	Properties map[string]any
	Revision   int64
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type NodeFilter struct {
	Domain DomainID
	Layer  string
	Limit  int
}

var slugRE = regexp.MustCompile(`^[a-z0-9-]+$`)

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
