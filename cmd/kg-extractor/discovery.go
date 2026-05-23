package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type discoveredPlugin struct {
	Dir      string
	Manifest *manifest
}

func discoverPlugins(path string) ([]discoveredPlugin, []error) {
	var plugins []discoveredPlugin
	var errs []error
	for _, root := range splitPath(path) {
		entries, err := os.ReadDir(root)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			errs = append(errs, fmt.Errorf("read %s: %w", root, err))
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			dir := filepath.Join(root, entry.Name())
			m, err := parseManifest(filepath.Join(dir, "manifest.json"))
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", entry.Name(), err))
				continue
			}
			plugins = append(plugins, discoveredPlugin{Dir: dir, Manifest: m})
		}
	}
	return plugins, errs
}

func splitPath(path string) []string {
	if path == "" {
		return nil
	}
	out := []string{}
	cur := ""
	for _, r := range path {
		if r == os.PathListSeparator {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
