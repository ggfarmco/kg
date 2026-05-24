package snapshot_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/snapshot"
)

func TestTopoSortParentsBeforeChildren(t *testing.T) {
	specs := []snapshot.NodeSpec{
		{ID: "d:a/b/c", Parent: "d:a/b", Layer: "decl", Name: "c"},
		{ID: "d:a", Layer: "package", Name: "a"},
		{ID: "d:a/b", Parent: "d:a", Layer: "file", Name: "b"},
	}
	sorted, err := snapshot.TopoSortNodes(specs)
	require.NoError(t, err)
	require.Equal(t, []string{"d:a", "d:a/b", "d:a/b/c"}, idsOf(sorted))
}

func TestTopoSortCycleErrors(t *testing.T) {
	specs := []snapshot.NodeSpec{
		{ID: "d:a", Parent: "d:b"},
		{ID: "d:b", Parent: "d:a"},
	}
	_, err := snapshot.TopoSortNodes(specs)
	require.Error(t, err)
}

func TestTopoSortParentOutsideSnapshotIsLeftAlone(t *testing.T) {
	specs := []snapshot.NodeSpec{
		{ID: "d:child", Parent: "d:external"},
		{ID: "d:root"},
	}
	sorted, err := snapshot.TopoSortNodes(specs)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"d:child", "d:root"}, idsOf(sorted))
}

func idsOf(specs []snapshot.NodeSpec) []string {
	out := make([]string, len(specs))
	for i, s := range specs {
		out[i] = s.ID
	}
	return out
}
