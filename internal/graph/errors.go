package graph

import "errors"

var (
	ErrDomainNotFound           = errors.New("domain not found")
	ErrDomainAlreadyExists      = errors.New("domain already exists")
	ErrLayerNotInDomain         = errors.New("layer not in domain")
	ErrSlugCannotDerive         = errors.New("cannot derive slug from name")
	ErrNodeNotFound             = errors.New("node not found")
	ErrNodeAlreadyExists        = errors.New("node already exists")
	ErrParentDomainMismatch     = errors.New("parent in different domain")
	ErrParentLayerMismatch      = errors.New("parent layer not one above")
	ErrTopLayerCannotHaveParent = errors.New("top-layer node cannot have parent")
	ErrEdgeSelfLoop             = errors.New("edge self-loop not allowed")
	ErrEdgeAlreadyExists        = errors.New("edge already exists")
	ErrEdgeNotFound             = errors.New("edge not found")
	ErrNestedTransaction        = errors.New("nested InTx is not supported")
	ErrHasDependents            = errors.New("entity has dependents")
	ErrInvalidSlug              = errors.New("invalid slug")

	ErrInvalidSourceID                = errors.New("invalid source id")
	ErrSourceNotFound                 = errors.New("source not found")
	ErrSourceHasDependents            = errors.New("source has dependents")
	ErrSourceRequired                 = errors.New("source is required")
	ErrNodeOwnedByDifferentSource     = errors.New("node owned by a different source")
	ErrNodeNotOwner                   = errors.New("not the owner of this node")
	ErrCoreFieldsImmutable            = errors.New("core node fields (layer/parent) are immutable")
	ErrNodeHasForeignClaims           = errors.New("node has incident edges with foreign claims")
	ErrDomainHasForeignWriters        = errors.New("domain contains nodes owned by other sources")
	ErrEdgeNoClaim                    = errors.New("edge has no claim from this source")
)
