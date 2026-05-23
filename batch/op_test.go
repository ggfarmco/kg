package batch_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/batch"
)

func TestOpNameConstants(t *testing.T) {
	require.Equal(t, batch.OpName("meta"), batch.OpMeta)
	require.Equal(t, batch.OpName("domain.add"), batch.OpDomainAdd)
	require.Equal(t, batch.OpName("node.add"), batch.OpNodeAdd)
	require.Equal(t, batch.OpName("node.update"), batch.OpNodeUpdate)
	require.Equal(t, batch.OpName("node.delete"), batch.OpNodeDelete)
	require.Equal(t, batch.OpName("edge.add"), batch.OpEdgeAdd)
	require.Equal(t, batch.OpName("edge.delete"), batch.OpEdgeDelete)
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

func TestNodeUpdateArgsDistinguishesAbsentFromEmpty(t *testing.T) {
	var out batch.NodeUpdateArgs
	require.NoError(t, json.Unmarshal([]byte(`{"id":"x:y"}`), &out))
	require.Nil(t, out.Name, "absent fields must stay nil")
	require.Nil(t, out.Summary)

	require.NoError(t, json.Unmarshal([]byte(`{"id":"x:y","name":""}`), &out))
	require.NotNil(t, out.Name)
	require.Equal(t, "", *out.Name)
}

func TestEdgeAddArgsRoundTrip(t *testing.T) {
	in := batch.EdgeAddArgs{Source: "a:b", Target: "a:c", Type: "imports", IfNotExists: true}
	b, err := json.Marshal(in)
	require.NoError(t, err)
	require.JSONEq(t, `{"source":"a:b","target":"a:c","type":"imports","if_not_exists":true}`, string(b))
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
