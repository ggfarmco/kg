package snapshot

import "encoding/json"

const ProtocolVersion = 2

type Scope string

const (
	ScopeDomainSource        Scope = "domain-source"
	ScopeDomain              Scope = "domain"
	ScopeAdditive            Scope = "additive"
)

type Snapshot struct {
	ProtocolVersion int         `json:"protocol_version"`
	Source          string      `json:"source"`
	Domain          string      `json:"domain"`
	Scope           Scope       `json:"scope"`
	DomainSpec      *DomainSpec `json:"domain_spec,omitempty"`
	Nodes           []NodeSpec  `json:"nodes"`
	Edges           []EdgeSpec  `json:"edges"`
}

type DomainSpec struct {
	ID          string   `json:"id"`
	Layers      []string `json:"layers"`
	Description string   `json:"description,omitempty"`
}

type NodeSpec struct {
	ID         string         `json:"id"`
	Layer      string         `json:"layer"`
	Parent     string         `json:"parent,omitempty"`
	Name       string         `json:"name"`
	Properties map[string]any `json:"properties,omitempty"`
}

type EdgeSpec struct {
	Src        string         `json:"-"`
	Target     string         `json:"target"`
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties,omitempty"`
}

type edgeSpecWire struct {
	Source     string         `json:"source,omitempty"`
	Src        string         `json:"src,omitempty"`
	Target     string         `json:"target"`
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties,omitempty"`
}

func (e EdgeSpec) MarshalJSON() ([]byte, error) {
	return json.Marshal(edgeSpecWire{
		Src: e.Src, Target: e.Target, Type: e.Type, Properties: e.Properties,
	})
}

func (e *EdgeSpec) UnmarshalJSON(b []byte) error {
	var w edgeSpecWire
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	e.Src = w.Src
	if e.Src == "" {
		e.Src = w.Source
	}
	e.Target = w.Target
	e.Type = w.Type
	e.Properties = w.Properties
	return nil
}
