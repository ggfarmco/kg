package graph

import "time"

type EdgeID int64

type Edge struct {
	ID         EdgeID                          `json:"id"`
	SourceID   NodeID                          `json:"source_id"`
	TargetID   NodeID                          `json:"target_id"`
	Type       string                          `json:"type"`
	Properties map[SourceID]map[string]any     `json:"properties"`
	Claims     []SourceID                      `json:"claims"`
	Revision   int64                           `json:"revision"`
	CreatedAt  time.Time                       `json:"created_at"`
}
