package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

type importResolver struct {
	modulePrefix string
	pkgBySuffix  map[string]string
}

func newImportResolver(input string, pkgs []*packageInfo) *importResolver {
	r := &importResolver{pkgBySuffix: map[string]string{}}
	modPath, modRoot := findModulePath(input)
	if modPath == "" {
		return r
	}
	inputAbs, err := filepath.Abs(input)
	if err != nil {
		return r
	}
	subdir, err := filepath.Rel(modRoot, inputAbs)
	if err != nil {
		return r
	}
	subdir = filepath.ToSlash(subdir)
	prefix := modPath
	if subdir != "" && subdir != "." {
		prefix = modPath + "/" + subdir
	}
	r.modulePrefix = prefix
	for _, p := range pkgs {
		suffix := p.RelDir
		key := prefix
		if suffix != "" {
			key = prefix + "/" + suffix
		}
		r.pkgBySuffix[key] = p.Slug
	}
	return r
}

func (r *importResolver) Resolve(importPath string) (slug string, internal bool) {
	if r == nil {
		return "", false
	}
	if slug, ok := r.pkgBySuffix[importPath]; ok {
		return slug, true
	}
	return "", false
}

func findModulePath(start string) (modPath, modRoot string) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", ""
	}
	for {
		f, err := os.Open(filepath.Join(dir, "go.mod"))
		if err == nil {
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if strings.HasPrefix(line, "module ") {
					modPath = strings.TrimSpace(strings.TrimPrefix(line, "module"))
					modPath = strings.Trim(modPath, "\"")
					f.Close()
					return modPath, dir
				}
			}
			f.Close()
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ""
		}
		dir = parent
	}
}
