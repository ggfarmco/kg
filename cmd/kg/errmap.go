package main

import (
	"errors"
	"strconv"
	"strings"

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
	case errors.Is(err, errExitOne):
		return mapped{1, "BATCH_PARTIAL", "", ""}
	case errors.As(err, new(parseErrSentinel)):
		return mapped{1, "INVALID_OP", err.Error(), ""}
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
	case errors.Is(err, graph.ErrSourceNotFound):
		return mapped{3, "SOURCE_NOT_FOUND", err.Error(), ""}
	case errors.Is(err, graph.ErrSourceAlreadyExists):
		return mapped{2, "SOURCE_ALREADY_EXISTS", err.Error(), "use --if-not-exists to skip silently"}
	case errors.Is(err, graph.ErrSourceHasDependents):
		return mapped{1, "SOURCE_HAS_DEPENDENTS", err.Error(), "delete owned nodes/claims first"}
	case errors.Is(err, graph.ErrSourceRequired):
		return mapped{1, "SOURCE_REQUIRED", err.Error(), "pass --source <id>"}
	case errors.Is(err, graph.ErrInvalidSourceID):
		return mapped{1, "INVALID_SOURCE_ID", err.Error(), ""}
	case errors.Is(err, graph.ErrNodeOwnedByDifferentSource):
		return mapped{2, "NODE_OWNED_BY_DIFFERENT_SOURCE", err.Error(), "another source owns this id; change yours or coordinate"}
	case errors.Is(err, graph.ErrNodeNotOwner):
		return mapped{1, "NODE_NOT_OWNER", err.Error(), "only the owning source can modify name/delete the node"}
	case errors.Is(err, graph.ErrCoreFieldsImmutable):
		return mapped{1, "CORE_FIELDS_IMMUTABLE", err.Error(), "layer/parent cannot change; delete and re-add"}
	case errors.Is(err, graph.ErrNodeHasForeignClaims):
		return mapped{1, "NODE_HAS_FOREIGN_CLAIMS", err.Error(), "re-run with --force-cascade to drop foreign claims, or keep the node alive"}
	case errors.Is(err, graph.ErrDomainHasForeignWriters):
		return mapped{1, "DOMAIN_FOREIGN_WRITERS", err.Error(), "narrow to --scope domain-source or pass --force-domain-takeover"}
	case strings.HasPrefix(err.Error(), "SOURCE_MISMATCH"):
		return mapped{1, "SOURCE_MISMATCH", err.Error(), ""}
	case strings.HasPrefix(err.Error(), "DOMAIN_MISMATCH"):
		return mapped{1, "DOMAIN_MISMATCH", err.Error(), ""}
	case errors.As(err, new(*strconv.NumError)):
		return mapped{1, "INVALID_INPUT", err.Error(), ""}
	default:
		return mapped{10, "INTERNAL", err.Error(), ""}
	}
}
