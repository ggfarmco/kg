package graph

import "context"

type Store interface {
	InTx(ctx context.Context, fn func(ctx context.Context) error) error

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

	UpsertEdge(ctx context.Context, e Edge) (EdgeID, error)
	GetEdge(ctx context.Context, id EdgeID) (*Edge, error)
	DeleteEdge(ctx context.Context, id EdgeID) error
	EdgesFrom(ctx context.Context, sourceID NodeID, types []string) ([]Edge, error)
	EdgesTo(ctx context.Context, targetID NodeID, types []string) ([]Edge, error)
}
