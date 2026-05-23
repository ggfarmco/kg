package batch_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/batch"
)

func TestOpNameWireValues(t *testing.T) {
	cases := map[batch.OpName]string{
		batch.OpMeta:       "meta",
		batch.OpDomainAdd:  "domain.add",
		batch.OpNodeAdd:    "node.add",
		batch.OpNodeUpdate: "node.update",
		batch.OpNodeDelete: "node.delete",
		batch.OpEdgeAdd:    "edge.add",
		batch.OpEdgeDelete: "edge.delete",
	}
	for got, want := range cases {
		require.Equal(t, want, string(got))
	}
}

func TestDomainAddArgsRoundTrip(t *testing.T) {
	in := batch.DomainAddArgs{
		ID:          "my-app",
		Layers:      []string{"package", "file", "decl"},
		Description: "...",
		IfNotExists: true,
	}
	b, err := json.Marshal(in)
	require.NoError(t, err)
	require.JSONEq(t, `{"id":"my-app","layers":["package","file","decl"],"description":"...","if_not_exists":true}`, string(b))

	var out batch.DomainAddArgs
	require.NoError(t, json.Unmarshal(b, &out))
	require.Equal(t, in, out)
}

func TestNodeAddArgsOmitsEmpty(t *testing.T) {
	in := batch.NodeAddArgs{Domain: "my-app", Layer: "package", Name: "fmt"}
	b, err := json.Marshal(in)
	require.NoError(t, err)
	require.JSONEq(t, `{"domain":"my-app","layer":"package","name":"fmt"}`, string(b))
}

func TestNodeUpdateArgsAbsentFieldsStayNil(t *testing.T) {
	var out batch.NodeUpdateArgs
	require.NoError(t, json.Unmarshal([]byte(`{"id":"x:y"}`), &out))
	require.Nil(t, out.Name)
	require.Nil(t, out.Summary)
}

func TestNodeUpdateArgsExplicitEmptyStringIsDistinguishable(t *testing.T) {
	var out batch.NodeUpdateArgs
	require.NoError(t, json.Unmarshal([]byte(`{"id":"x:y","name":""}`), &out))
	require.NotNil(t, out.Name)
	require.Equal(t, "", *out.Name)
}

func TestEdgeDeleteArgsUsesInt(t *testing.T) {
	in := batch.EdgeDeleteArgs{ID: 42}
	b, err := json.Marshal(in)
	require.NoError(t, err)
	require.JSONEq(t, `{"id":42}`, string(b))
}

func TestMetaArgsTotalOpsOptional(t *testing.T) {
	var out batch.MetaArgs
	require.NoError(t, json.Unmarshal([]byte(`{"plugin":"x"}`), &out))
	require.Equal(t, "x", out.Plugin)
	require.Zero(t, out.TotalOps)
}
