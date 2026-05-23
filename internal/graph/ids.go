package graph

import (
	"errors"
	"regexp"
	"strings"
)

type (
	DomainID string
	SlugID   string
	NodeID   string
	EdgeID   int64
)

var ErrInvalidSlug = errors.New("invalid slug")

var slugRE = regexp.MustCompile(`^[a-z0-9-]+$`)

func ParseSlug(s string) (SlugID, error) {
	if !slugRE.MatchString(s) {
		return "", ErrInvalidSlug
	}
	return SlugID(s), nil
}

func ParseDomainID(s string) (DomainID, error) {
	if !slugRE.MatchString(s) {
		return "", ErrInvalidSlug
	}
	return DomainID(s), nil
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
