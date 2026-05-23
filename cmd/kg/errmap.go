package main

import (
	"errors"
	"strconv"

	"github.com/ggfarmco/kg/internal/graph"
)

type mapped struct {
	exit    int
	code    string
	message string
	hint    string
}

func mapError(err error) mapped {
	switch {
	case errors.Is(err, graph.ErrDomainNotFound):
		return mapped{3, "DOMAIN_NOT_FOUND", err.Error(), "run `kg domain list`"}
	case errors.Is(err, graph.ErrNodeNotFound):
		return mapped{3, "NODE_NOT_FOUND", err.Error(), "run `kg node list` to find existing IDs"}
	case errors.Is(err, graph.ErrEdgeNotFound):
		return mapped{3, "EDGE_NOT_FOUND", err.Error(), ""}
	case errors.Is(err, graph.ErrDomainAlreadyExists):
		return mapped{2, "DOMAIN_ALREADY_EXISTS", err.Error(), "use --if-not-exists to skip silently"}
	case errors.Is(err, graph.ErrNodeAlreadyExists):
		return mapped{2, "NODE_ALREADY_EXISTS", err.Error(), "use --if-not-exists to skip silently"}
	case errors.Is(err, graph.ErrEdgeAlreadyExists):
		return mapped{2, "EDGE_ALREADY_EXISTS", err.Error(), "use --if-not-exists to skip silently"}
	case errors.Is(err, graph.ErrInvalidSlug):
		return mapped{1, "INVALID_SLUG", err.Error(), "slugs must match ^[a-z0-9-]+$"}
	case errors.Is(err, graph.ErrSlugCannotDerive):
		return mapped{1, "SLUG_CANNOT_DERIVE", err.Error(), "pass --id explicitly"}
	case errors.Is(err, graph.ErrLayerNotInDomain):
		return mapped{1, "LAYER_NOT_IN_DOMAIN", err.Error(), ""}
	case errors.Is(err, graph.ErrParentDomainMismatch):
		return mapped{1, "PARENT_DOMAIN_MISMATCH", err.Error(), ""}
	case errors.Is(err, graph.ErrParentLayerMismatch):
		return mapped{1, "PARENT_LAYER_MISMATCH", err.Error(), ""}
	case errors.Is(err, graph.ErrTopLayerCannotHaveParent):
		return mapped{1, "TOP_LAYER_CANNOT_HAVE_PARENT", err.Error(), ""}
	case errors.Is(err, graph.ErrEdgeSelfLoop):
		return mapped{1, "EDGE_SELF_LOOP", err.Error(), ""}
	case errors.Is(err, graph.ErrHasDependents):
		return mapped{1, "HAS_DEPENDENTS", err.Error(), "remove children first, or use a future --cascade flag"}
	case errors.As(err, new(*strconv.NumError)):
		return mapped{1, "INVALID_INPUT", err.Error(), ""}
	case errors.Is(err, errExitOne):
		return mapped{1, "BATCH_PARTIAL", "", ""}
	case errors.As(err, new(parseErrSentinel)):
		return mapped{1, "INVALID_OP", err.Error(), ""}
	default:
		return mapped{10, "INTERNAL", err.Error(), ""}
	}
}
