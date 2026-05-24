package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/snapshot"
)

func TestBuildSnapshotIncludesPackagesFilesDecls(t *testing.T) {
	pkgs := []*packageInfo{
		{
			Path: "graph", Slug: "graph", RelDir: "graph",
			Files: []fileInfo{
				{AbsPath: "/abs/graph/node.go", RelPath: "graph/node.go", BasenameSlug: "node-go",
					Decls: []Decl{{NameSlug: "parseslug", Properties: map[string]any{"kind": "function"}}}},
			},
		},
	}
	res := newImportResolver("/tmp/x", pkgs)
	snap := buildSnapshot("go", "myapp", pkgs, res, false)

	require.Equal(t, snapshot.ProtocolVersion, snap.ProtocolVersion)
	require.Equal(t, "tree-sitter:0.2.0", snap.Source)
	require.Equal(t, "myapp", snap.Domain)
	require.Equal(t, snapshot.ScopeDomainSource, snap.Scope)
	byID := map[string]snapshot.NodeSpec{}
	for _, n := range snap.Nodes {
		byID[n.ID] = n
	}
	require.Contains(t, byID, "myapp:graph")
	require.Contains(t, byID, "myapp:graph/node-go")
	require.Contains(t, byID, "myapp:graph/node-go::parseslug")
	require.Equal(t, "/abs/graph/node.go", byID["myapp:graph/node-go"].Properties["path"], "file-layer node carries abs path")
	require.Equal(t, "graph/node.go", byID["myapp:graph/node-go"].Properties["rel_path"], "file-layer node carries rel path")
}

func TestBuildSnapshotSkipsExternalImportsByDefault(t *testing.T) {
	pkgs := []*packageInfo{
		{Path: "graph", Slug: "graph", RelDir: "graph",
			Imports: []Import{{From: "graph", To: "github.com/external/x"}}},
	}
	res := newImportResolver("/tmp/x", pkgs)
	snap := buildSnapshot("go", "myapp", pkgs, res, false)
	require.Empty(t, snap.Edges, "external imports skipped when include_external_imports=false")
}
