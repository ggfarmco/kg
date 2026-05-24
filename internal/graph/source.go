package graph

import (
	"regexp"
	"time"
)

type SourceID string

type Source struct {
	ID          SourceID  `json:"id"`
	Description string    `json:"description"`
	Trust       int       `json:"trust"`
	FirstSeen   time.Time `json:"first_seen"`
	LastSeen    time.Time `json:"last_seen"`
}

type EdgeClaim struct {
	EdgeID    EdgeID    `json:"edge_id"`
	Source    SourceID  `json:"source"`
	ClaimedAt time.Time `json:"claimed_at"`
}

var sourceIDRE = regexp.MustCompile(`^[a-z0-9-]+(?:/[a-z0-9-]+)*(?::[a-z0-9][a-z0-9.\-]*)?$`)

func ParseSourceID(s string) (SourceID, error) {
	if !sourceIDRE.MatchString(s) {
		return "", ErrInvalidSourceID
	}
	return SourceID(s), nil
}
