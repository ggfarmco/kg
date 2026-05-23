package graph

import "time"

type EdgeID int64

type Edge struct {
	ID         EdgeID
	SourceID   NodeID
	TargetID   NodeID
	Type       string
	Properties map[string]any
	Revision   int64
	CreatedAt  time.Time
}
