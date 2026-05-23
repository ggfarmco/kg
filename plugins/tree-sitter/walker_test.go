package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWalkPackages(t *testing.T) {
	root := t.TempDir()
	mkFile(t, root, "a/x.go", "package a\n")
	mkFile(t, root, "a/y.go", "package a\n")
	mkFile(t, root, "a/sub/z.go", "package sub\n")
	mkFile(t, root, "vendor/skip/v.go", "package skip\n")
	mkFile(t, root, ".git/skip/g.go", "package skip\n")
	mkFile(t, root, "a/x_test.go", "package a\n")

	pkgs, err := walkPackages(root, []string{".go"}, true)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"a", "a/sub"}, pathsOf(pkgs))

	pkgA := findPkg(pkgs, "a")
	require.Len(t, pkgA.Files, 2, "test file excluded")
}

func TestWalkPackagesIncludesTestFiles(t *testing.T) {
	root := t.TempDir()
	mkFile(t, root, "a/x.go", "package a\n")
	mkFile(t, root, "a/x_test.go", "package a\n")

	pkgs, err := walkPackages(root, []string{".go"}, false)
	require.NoError(t, err)
	require.Len(t, findPkg(pkgs, "a").Files, 2)
}

func mkFile(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
}

func pathsOf(pkgs []*packageInfo) []string {
	out := make([]string, 0, len(pkgs))
	for _, p := range pkgs {
		out = append(out, p.Path)
	}
	return out
}

func findPkg(pkgs []*packageInfo, path string) *packageInfo {
	for _, p := range pkgs {
		if p.Path == path {
			return p
		}
	}
	return nil
}
