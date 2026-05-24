package snapshot_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/snapshot"
)

func TestValidateRejectsWrongProtocol(t *testing.T) {
	err := snapshot.Validate(&snapshot.Snapshot{ProtocolVersion: 1, Source: "x", Domain: "d", Scope: "domain-source"})
	require.ErrorIs(t, err, snapshot.ErrProtocolVersion)
}

func TestValidateRejectsUnknownScope(t *testing.T) {
	err := snapshot.Validate(&snapshot.Snapshot{ProtocolVersion: 2, Source: "x", Domain: "d", Scope: "wat"})
	require.ErrorIs(t, err, snapshot.ErrUnknownScope)
}

func TestValidateRejectsBadNodeID(t *testing.T) {
	err := snapshot.Validate(&snapshot.Snapshot{
		ProtocolVersion: 2, Source: "x", Domain: "d", Scope: "domain-source",
		Nodes: []snapshot.NodeSpec{{ID: "BAD CASE", Layer: "l", Name: "n"}},
	})
	require.ErrorIs(t, err, snapshot.ErrInvalidNodeID)
}

func TestValidateAcceptsRelaxedCompoundSlug(t *testing.T) {
	err := snapshot.Validate(&snapshot.Snapshot{
		ProtocolVersion: 2, Source: "x", Domain: "d", Scope: "domain-source",
		Nodes: []snapshot.NodeSpec{{ID: "d:a/b-go::parseslug", Layer: "decl", Name: "ParseSlug", Parent: "d:a/b-go"}},
		Edges: []snapshot.EdgeSpec{},
	})
	require.NoError(t, err)
}

func TestValidateAdditiveAllowsBareNodeID(t *testing.T) {
	err := snapshot.Validate(&snapshot.Snapshot{
		ProtocolVersion: 2, Source: "b", Domain: "d", Scope: snapshot.ScopeAdditive,
		Nodes:           []snapshot.NodeSpec{{ID: "d:x", Properties: map[string]any{"k": "v"}}},
	})
	require.NoError(t, err)
}

func TestValidateDomainSourceStillRequiresLayerAndName(t *testing.T) {
	for _, scope := range []snapshot.Scope{snapshot.ScopeDomainSource, snapshot.ScopeDomain} {
		err := snapshot.Validate(&snapshot.Snapshot{
			ProtocolVersion: 2, Source: "b", Domain: "d", Scope: scope,
			Nodes:           []snapshot.NodeSpec{{ID: "d:x"}},
		})
		require.Error(t, err, "scope=%s should require layer+name", scope)
	}
}
