package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var skipDirs = map[string]bool{
	"vendor":       true,
	".git":         true,
	"node_modules": true,
}

func walkPackages(root string, extensions []string, skipTests bool) ([]*packageInfo, error) {
	byPath := map[string]*packageInfo{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !hasAnyExt(d.Name(), extensions) {
			return nil
		}
		if skipTests && strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relDir := filepath.ToSlash(filepath.Dir(rel))
		pkgPath := relDir
		if pkgPath == "." {
			pkgPath = filepath.Base(root)
			relDir = ""
		}
		pkg, ok := byPath[pkgPath]
		if !ok {
			pkg = &packageInfo{
				Path:     pkgPath,
				RelDir:   relDir,
				Slug:     sanitizeSlug(pkgPath),
				DeclByID: map[string]struct{}{},
			}
			byPath[pkgPath] = pkg
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		base := strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))
		pkg.Files = append(pkg.Files, fileInfo{
			AbsPath:      path,
			RelPath:      filepath.ToSlash(rel),
			BasenameSlug: sanitizeSlug(base),
			PackagePath:  pkgPath,
			Source:       src,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]*packageInfo, 0, len(byPath))
	for _, p := range byPath {
		sort.Slice(p.Files, func(i, j int) bool { return p.Files[i].RelPath < p.Files[j].RelPath })
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func hasAnyExt(name string, exts []string) bool {
	for _, e := range exts {
		if strings.HasSuffix(name, e) {
			return true
		}
	}
	return false
}
