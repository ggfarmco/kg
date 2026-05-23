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

func TestParseSlugAcceptsCompoundForms(t *testing.T) {
	cases := []string{
		"engine",
		"graph/node-go",
		"graph/node-go::parseslug",
		"a/b/c",
		"a::b::c",
		"a/b::c/d::e",
	}
	for _, in := range cases {
		_, err := graph.ParseSlug(in)
		require.NoError(t, err, "expected %q to be a valid slug", in)
	}
}

func TestParseSlugRejectsBadCompounds(t *testing.T) {
	cases := []string{
		"with:colon",
		"/foo",
		"foo/",
		"foo//bar",
		"foo::::bar",
		"foo/::bar",
	}
	for _, in := range cases {
		_, err := graph.ParseSlug(in)
		require.ErrorIs(t, err, graph.ErrInvalidSlug, "expected %q to be invalid", in)
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
