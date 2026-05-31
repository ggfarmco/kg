package snapshot

import (
	"errors"
	"fmt"
	"regexp"
)

var (
	ErrProtocolVersion = errors.New("unsupported protocol_version (expected 2)")
	ErrUnknownScope    = errors.New("unknown scope")
	ErrInvalidNodeID   = errors.New("invalid node id (must match kg slug grammar)")
	ErrDuplicateNodeID = errors.New("duplicate node id")
	ErrMissingSource   = errors.New("source is required")
	ErrMissingDomain   = errors.New("domain is required")
)

var nodeIDRE = regexp.MustCompile(`^[a-z0-9-]+:[a-z0-9-]+(?:(?:/|::)[a-z0-9-]+)*$`)

func Validate(s *Snapshot) error {
	if s.ProtocolVersion != ProtocolVersion {
		return fmt.Errorf("%w: got %d", ErrProtocolVersion, s.ProtocolVersion)
	}
	if s.Source == "" {
		return ErrMissingSource
	}
	if s.Domain == "" {
		return ErrMissingDomain
	}
	switch s.Scope {
	case ScopeDomainSource, ScopeDomain, ScopeAdditive:
	default:
		return fmt.Errorf("%w: %q", ErrUnknownScope, s.Scope)
	}
	seen := make(map[string]struct{}, len(s.Nodes))
	for i, n := range s.Nodes {
		if !nodeIDRE.MatchString(n.ID) {
			return fmt.Errorf("%w: nodes[%d].id=%q", ErrInvalidNodeID, i, n.ID)
		}
		if _, dup := seen[n.ID]; dup {
			return fmt.Errorf("%w: nodes[%d].id=%q", ErrDuplicateNodeID, i, n.ID)
		}
		seen[n.ID] = struct{}{}
		if s.Scope != ScopeAdditive {
			if n.Layer == "" || n.Name == "" {
				return fmt.Errorf("nodes[%d]: layer and name are required", i)
			}
		}
	}
	for i, e := range s.Edges {
		if e.Src == "" || e.Target == "" || e.Type == "" {
			return fmt.Errorf("edges[%d]: src/target/type are all required", i)
		}
		if !nodeIDRE.MatchString(e.Src) || !nodeIDRE.MatchString(e.Target) {
			return fmt.Errorf("%w: edges[%d] endpoints", ErrInvalidNodeID, i)
		}
	}
	return nil
}
