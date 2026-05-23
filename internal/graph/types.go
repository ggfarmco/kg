package graph

import "time"

type Domain struct {
	ID          DomainID
	Description string
	Layers      []string
	Revision    int64
	CreatedAt   time.Time
}

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

type Edge struct {
	ID         EdgeID
	SourceID   NodeID
	TargetID   NodeID
	Type       string
	Properties map[string]any
	Revision   int64
	CreatedAt  time.Time
}

type NodeFilter struct {
	Domain DomainID
	Layer  string
	Limit  int
}
