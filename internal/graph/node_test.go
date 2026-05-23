package graph_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/internal/graph"
)

func TestParseSlug(t *testing.T) {
	cases := []struct {
		in      string
		want    graph.SlugID
		wantErr bool
	}{
		{"engine", "engine", false},
		{"v8-engine", "v8-engine", false},
		{"v8", "v8", false},
		{"Engine", "", true},
		{"", "", true},
		{"with space", "", true},
		{"with:colon", "", true},
	}
	for _, tc := range cases {
		got, err := graph.ParseSlug(tc.in)
		if tc.wantErr {
			require.ErrorIs(t, err, graph.ErrInvalidSlug, "input=%q", tc.in)
			continue
		}
		require.NoError(t, err, "input=%q", tc.in)
		require.Equal(t, tc.want, got)
	}
}

func TestNodeIDRoundtrip(t *testing.T) {
	id := graph.NewNodeID("cars", "engine")
	require.Equal(t, graph.NodeID("cars:engine"), id)

	d, s, err := id.Split()
	require.NoError(t, err)
	require.Equal(t, graph.DomainID("cars"), d)
	require.Equal(t, graph.SlugID("engine"), s)
}

func TestNodeIDSplitInvalid(t *testing.T) {
	_, _, err := graph.NodeID("no-colon").Split()
	require.Error(t, err)

	_, _, err = graph.NodeID("Cars:engine").Split()
	require.ErrorIs(t, err, graph.ErrInvalidSlug)
}
