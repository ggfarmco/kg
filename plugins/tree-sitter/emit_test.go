package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEmitProducesOrderedOps(t *testing.T) {
	pkg := &packageInfo{
		Path:       "a",
		Slug:       "a",
		DeclByID:   map[string]struct{}{},
		Properties: map[string]any{"import_path": "x/a"},
		Files: []fileInfo{
			{
				BasenameSlug: "x-go",
				RelPath:      "a/x.go",
				Decls: []Decl{
					{NameSlug: "foo", Properties: map[string]any{"kind": "function"}},
				},
			},
		},
		Imports: []Import{{From: "a", To: "b"}},
		Calls:   []Call{},
	}
	pkgB := &packageInfo{Path: "b", Slug: "b", DeclByID: map[string]struct{}{}}

	resolver := &importResolver{pkgBySuffix: map[string]string{"b": "b"}}
	var buf bytes.Buffer
	require.NoError(t, emitOps(&buf, "lang", "demo", []*packageInfo{pkg, pkgB}, resolver, false))

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	require.GreaterOrEqual(t, len(lines), 6)
	require.Contains(t, lines[0], `"meta"`)
	require.Contains(t, lines[1], `"domain.add"`)
	require.Contains(t, lines[2], `"node.add"`)
	require.Contains(t, lines[2], `"layer":"package"`)
	require.Contains(t, buf.String(), `"layer":"file"`)
	require.Contains(t, buf.String(), `"layer":"decl"`)
	require.Contains(t, buf.String(), `"imports"`)
}
