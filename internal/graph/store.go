package graph

import (
	"context"
	"time"
)

type Store interface {
	InTx(ctx context.Context, fn func(ctx context.Context) error) error

	UpsertSource(ctx context.Context, src Source) error
	GetSource(ctx context.Context, id SourceID) (*Source, error)
	ListSources(ctx context.Context) ([]Source, error)
	UpdateSource(ctx context.Context, src Source) error
	DeleteSource(ctx context.Context, id SourceID) error

	CreateDomain(ctx context.Context, d Domain) error
	GetDomain(ctx context.Context, id DomainID) (*Domain, error)
	ListDomains(ctx context.Context) ([]Domain, error)
	DeleteDomain(ctx context.Context, id DomainID) error

	CreateNode(ctx context.Context, n Node) error
	GetNode(ctx context.Context, id NodeID) (*Node, error)
	ListNodes(ctx context.Context, filter NodeFilter) ([]Node, error)
	UpdateNode(ctx context.Context, n Node) error
	DeleteNode(ctx context.Context, id NodeID) error
	ChildrenOf(ctx context.Context, parentID NodeID) ([]Node, error)
	NodesOwnedBy(ctx context.Context, domain DomainID, source SourceID) ([]Node, error)

	UpsertEdge(ctx context.Context, e Edge) (EdgeID, error)
	GetEdge(ctx context.Context, id EdgeID) (*Edge, error)
	UpdateEdgeProperties(ctx context.Context, id EdgeID, props map[SourceID]map[string]any) error
	DeleteEdge(ctx context.Context, id EdgeID) error
	EdgesFrom(ctx context.Context, sourceID NodeID, types []string) ([]Edge, error)
	EdgesTo(ctx context.Context, targetID NodeID, types []string) ([]Edge, error)

	AddEdgeClaim(ctx context.Context, edgeID EdgeID, source SourceID, at time.Time) error
	RemoveEdgeClaim(ctx context.Context, edgeID EdgeID, source SourceID) error
	CountEdgeClaims(ctx context.Context, edgeID EdgeID) (int, error)
	ListEdgeClaims(ctx context.Context, edgeID EdgeID) ([]EdgeClaim, error)
	EdgeIDsClaimedBy(ctx context.Context, source SourceID) ([]EdgeID, error)
}
