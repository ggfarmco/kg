package graph

import "time"

type DomainID string

type Domain struct {
	ID          DomainID  `json:"id"`
	Description string    `json:"description"`
	Layers      []string  `json:"layers"`
	Revision    int64     `json:"revision"`
	CreatedAt   time.Time `json:"created_at"`
}

func ParseDomainID(s string) (DomainID, error) {
	if !slugRE.MatchString(s) {
		return "", ErrInvalidSlug
	}
	return DomainID(s), nil
}
