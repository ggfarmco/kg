package batch

import "encoding/json"

type OpName string

const (
	OpMeta       OpName = "meta"
	OpDomainAdd  OpName = "domain.add"
	OpNodeAdd    OpName = "node.add"
	OpNodeUpdate OpName = "node.update"
	OpNodeDelete OpName = "node.delete"
	OpEdgeAdd    OpName = "edge.add"
	OpEdgeDelete OpName = "edge.delete"
)

const ProtocolVersion = 1

type Op struct {
	Op   OpName          `json:"op"`
	Args json.RawMessage `json:"args"`
}

type MetaArgs struct {
	Plugin   string `json:"plugin,omitempty"`
	Version  string `json:"version,omitempty"`
	Language string `json:"language,omitempty"`
	TotalOps int64  `json:"total_ops,omitempty"`
}

type DomainAddArgs struct {
	ID          string         `json:"id"`
	Layers      []string       `json:"layers"`
	Description string         `json:"description,omitempty"`
	Source      string         `json:"source,omitempty"`
	Properties  map[string]any `json:"properties,omitempty"`
	IfNotExists bool           `json:"if_not_exists,omitempty"`
}

type NodeAddArgs struct {
	Domain      string         `json:"domain"`
	Layer       string         `json:"layer"`
	Name        string         `json:"name"`
	ID          string         `json:"id,omitempty"`
	Parent      string         `json:"parent,omitempty"`
	Source      string         `json:"source,omitempty"`
	Summary     string         `json:"summary,omitempty"`
	Properties  map[string]any `json:"properties,omitempty"`
	IfNotExists bool           `json:"if_not_exists,omitempty"`
}

type NodeUpdateArgs struct {
	ID      string  `json:"id"`
	Source  string  `json:"source,omitempty"`
	Name    *string `json:"name,omitempty"`
	Summary *string `json:"summary,omitempty"`
}

type NodeDeleteArgs struct {
	ID     string `json:"id"`
	Source string `json:"source,omitempty"`
}

type EdgeAddArgs struct {
	Source       string         `json:"source"`
	Target       string         `json:"target"`
	Type         string         `json:"type"`
	WriterSource string         `json:"writer_source,omitempty"`
	Properties   map[string]any `json:"properties,omitempty"`
	IfNotExists  bool           `json:"if_not_exists,omitempty"`
}

type EdgeDeleteArgs struct {
	ID int64 `json:"id"`
}

func IsKnownOp(name OpName) bool {
	switch name {
	case OpMeta, OpDomainAdd, OpNodeAdd, OpNodeUpdate, OpNodeDelete, OpEdgeAdd, OpEdgeDelete:
		return true
	}
	return false
}
