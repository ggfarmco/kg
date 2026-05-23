package main

import (
	"fmt"
	"io"
	"sort"

	"github.com/ggfarmco/kg/batch"
)

func emitOps(w io.Writer, language, domain string, pkgs []*packageInfo) error {
	enc := batch.NewEncoder(w)

	sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].Path < pkgs[j].Path })

	var total int64
	total += 1 + 1
	for _, p := range pkgs {
		total++
		for _, f := range p.Files {
			total++
			total += int64(len(f.Decls))
		}
		total += int64(len(p.Imports))
		total += int64(len(p.Calls))
	}

	if err := enc.Meta(batch.MetaArgs{Plugin: "tree-sitter", Language: language, TotalOps: total}); err != nil {
		return err
	}
	if err := enc.DomainAdd(batch.DomainAddArgs{
		ID:          domain,
		Layers:      []string{"package", "file", "decl"},
		Description: "Extracted by kg-extractor-tree-sitter (" + language + ")",
		IfNotExists: true,
	}); err != nil {
		return err
	}

	for _, p := range pkgs {
		pkgID := domain + ":" + p.Slug
		if err := enc.NodeAdd(batch.NodeAddArgs{
			Domain: domain, Layer: "package", Name: p.Path, ID: p.Slug,
			Properties: nonNilMap(p.Properties), IfNotExists: true,
		}); err != nil {
			return err
		}
		for _, f := range p.Files {
			fileSlug := p.Slug + "/" + f.BasenameSlug
			if err := enc.NodeAdd(batch.NodeAddArgs{
				Domain: domain, Layer: "file", Name: f.RelPath, ID: fileSlug,
				Parent:      pkgID,
				IfNotExists: true,
			}); err != nil {
				return err
			}
			for _, d := range f.Decls {
				declSlug := fileSlug + "::" + d.NameSlug
				if err := enc.NodeAdd(batch.NodeAddArgs{
					Domain: domain, Layer: "decl", Name: d.NameSlug, ID: declSlug,
					Parent:      domain + ":" + fileSlug,
					Properties:  nonNilMap(d.Properties),
					IfNotExists: true,
				}); err != nil {
					return err
				}
			}
		}
	}

	for _, p := range pkgs {
		for _, imp := range p.Imports {
			if err := enc.EdgeAdd(batch.EdgeAddArgs{
				Source:      domain + ":" + sanitizeSlug(imp.From),
				Target:      domain + ":" + sanitizeSlug(imp.To),
				Type:        "imports",
				IfNotExists: true,
			}); err != nil {
				return err
			}
		}
	}
	for _, p := range pkgs {
		for _, call := range p.Calls {
			if err := enc.EdgeAdd(batch.EdgeAddArgs{
				Source:      domain + ":" + call.FromDecl,
				Target:      domain + ":" + call.ToDecl,
				Type:        "calls",
				IfNotExists: true,
			}); err != nil {
				return err
			}
		}
	}

	if total < 0 {
		return fmt.Errorf("negative total: %d", total)
	}
	return nil
}

func nonNilMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	return m
}
