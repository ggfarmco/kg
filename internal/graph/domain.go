package graph

import "time"

type DomainID string

type Domain struct {
	ID          DomainID
	Description string
	Layers      []string
	Revision    int64
	CreatedAt   time.Time
}

func ParseDomainID(s string) (DomainID, error) {
	if !slugRE.MatchString(s) {
		return "", ErrInvalidSlug
	}
	return DomainID(s), nil
}
