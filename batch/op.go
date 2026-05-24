package batch

import "encoding/json"

type OpName string

const (
	OpMeta        OpName = "meta"
	OpDomainAdd   OpName = "domain.add"
	OpNodeAdd     OpName = "node.add"
	OpNodeUpdate  OpName = "node.update"
	OpNodeDelete  OpName = "node.delete"
	OpEdgeAdd     OpName = "edge.add"
	OpEdgeDelete  OpName = "edge.delete"
	OpEdgeUnclaim OpName = "edge.unclaim"
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
	Properties  map[string]any `json:"properties,omitempty"`
	Source      string         `json:"source"`
	IfNotExists bool           `json:"if_not_exists,omitempty"`
}

type NodeAddArgs struct {
	Domain      string         `json:"domain"`
	Layer       string         `json:"layer"`
	Name        string         `json:"name"`
	ID          string         `json:"id,omitempty"`
	Parent      string         `json:"parent,omitempty"`
	Source      string         `json:"source"`
	Properties  map[string]any `json:"properties,omitempty"`
	IfNotExists bool           `json:"if_not_exists,omitempty"`
}

type NodeUpdateArgs struct {
	ID         string         `json:"id"`
	Source     string         `json:"source"`
	Name       *string        `json:"name,omitempty"`
	Properties map[string]any `json:"properties,omitempty"`
}

type NodeDeleteArgs struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Force  bool   `json:"force,omitempty"`
}

type EdgeAddArgs struct {
	Src         string         `json:"src"`
	Target      string         `json:"target"`
	Type        string         `json:"type"`
	Source      string         `json:"source"`
	Properties  map[string]any `json:"properties,omitempty"`
	IfNotExists bool           `json:"if_not_exists,omitempty"`
}

func (a *EdgeAddArgs) UnmarshalJSON(b []byte) error {
	var raw struct {
		Src         string         `json:"src"`
		Target      string         `json:"target"`
		Type        string         `json:"type"`
		Source      string         `json:"source"`
		WriterSource string        `json:"writer_source,omitempty"`
		Properties  map[string]any `json:"properties,omitempty"`
		IfNotExists bool           `json:"if_not_exists,omitempty"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	a.Src = raw.Src
	a.Target = raw.Target
	a.Type = raw.Type
	a.Source = raw.Source
	a.Properties = raw.Properties
	a.IfNotExists = raw.IfNotExists
	if a.Src == "" && raw.Source != "" && raw.WriterSource == "" {
		// Legacy v1 wire: "source" was the originating node; no writer_source field existed.
		a.Src = raw.Source
		a.Source = ""
	}
	if a.Source == "" && raw.WriterSource != "" {
		a.Source = raw.WriterSource
	}
	return nil
}

type EdgeDeleteArgs struct {
	ID     int64  `json:"id"`
	Source string `json:"source,omitempty"`
	Force  bool   `json:"force,omitempty"`
}

type EdgeUnclaimArgs struct {
	ID     int64  `json:"id"`
	Source string `json:"source"`
}

func IsKnownOp(name OpName) bool {
	switch name {
	case OpMeta, OpDomainAdd, OpNodeAdd, OpNodeUpdate, OpNodeDelete, OpEdgeAdd, OpEdgeDelete, OpEdgeUnclaim:
		return true
	}
	return false
}
