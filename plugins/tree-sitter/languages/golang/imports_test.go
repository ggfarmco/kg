package golang

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func importsOf(c *captureAll) []string {
	out := make([]string, 0, len(c.imports))
	for _, i := range c.imports {
		out = append(out, i[1])
	}
	return out
}

func TestExtractSingleImport(t *testing.T) {
	src := []byte(`package x

import "fmt"
`)
	g := New()
	sinks := &captureAll{src: src}
	require.NoError(t, g.ExtractFile(context.Background(), sinks, sinks, sinks))
	require.Equal(t, []string{"fmt"}, importsOf(sinks))
}

func TestExtractGroupedImports(t *testing.T) {
	src := []byte(`package x

import (
	"fmt"
	"io"
)
`)
	g := New()
	sinks := &captureAll{src: src}
	require.NoError(t, g.ExtractFile(context.Background(), sinks, sinks, sinks))
	require.ElementsMatch(t, []string{"fmt", "io"}, importsOf(sinks))
}

func TestExtractAliasedImport(t *testing.T) {
	src := []byte(`package x

import alt "io"
`)
	g := New()
	sinks := &captureAll{src: src}
	require.NoError(t, g.ExtractFile(context.Background(), sinks, sinks, sinks))
	require.Equal(t, []string{"io"}, importsOf(sinks), "alias does not change the import path")
}
