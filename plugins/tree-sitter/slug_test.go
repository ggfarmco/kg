package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSanitizeSlug(t *testing.T) {
	cases := []struct{ in, want string }{
		{"internal/graph", "internal-graph"},
		{"Node.go", "node-go"},
		{"tree-sitter", "tree-sitter"},
		{"__init__", "init"},
		{"My Class!", "my-class"},
		{"   ", ""},
		{"123abc", "123abc"},
		{"a---b", "a-b"},
		{"camelCase", "camelcase"},
	}
	for _, tc := range cases {
		require.Equal(t, tc.want, sanitizeSlug(tc.in), "input=%q", tc.in)
	}
}
