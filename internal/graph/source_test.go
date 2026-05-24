package graph_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/internal/graph"
)

func TestParseSourceID(t *testing.T) {
	cases := map[string]bool{
		"cli":               true,
		"manual":            true,
		"tree-sitter:0.1.0": true,
		"acme/foo:1.0":      true,
		"":                  false,
		"Bad ID":            false,
		"colon::only":       false,
	}
	for in, ok := range cases {
		_, err := graph.ParseSourceID(in)
		if ok {
			require.NoError(t, err, "want valid: %q", in)
		} else {
			require.Error(t, err, "want invalid: %q", in)
		}
	}
}
