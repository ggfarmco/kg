package main

import (
	"fmt"
	"io"
	"sort"

	"github.com/ggfarmco/kg/batch"
)

func emitOps(w io.Writer, language, domain string, pkgs []*packageInfo, resolver *importResolver, includeExternalImports bool) error {
	enc := batch.NewEncoder(w)

	sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].Path < pkgs[j].Path })

	type resolvedImport struct {
		fromSlug string
		toSlug   string
	}
	type externalPkg struct {
		slug       string
		importPath string
	}
	resolvedImports := make([][]resolvedImport, len(pkgs))
	externalsByID := map[string]externalPkg{}
	var externalsOrder []string
	for i, p := range pkgs {
		for _, imp := range p.Imports {
			toSlug, internal := resolver.Resolve(imp.To)
			if !internal {
				if !includeExternalImports {
					continue
				}
				toSlug = "ext-" + sanitizeSlug(imp.To)
				if _, ok := externalsByID[toSlug]; !ok {
					externalsByID[toSlug] = externalPkg{slug: toSlug, importPath: imp.To}
					externalsOrder = append(externalsOrder, toSlug)
				}
			}
			resolvedImports[i] = append(resolvedImports[i], resolvedImport{
				fromSlug: sanitizeSlug(imp.From),
				toSlug:   toSlug,
			})
		}
	}

	var total int64
	total += 1 + 1
	for _, p := range pkgs {
		total++
		for _, f := range p.Files {
			total++
			total += int64(len(f.Decls))
		}
		total += int64(len(p.Calls))
	}
	for _, ri := range resolvedImports {
		total += int64(len(ri))
	}
	total += int64(len(externalsOrder))

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

	for _, slug := range externalsOrder {
		ext := externalsByID[slug]
		if err := enc.NodeAdd(batch.NodeAddArgs{
			Domain: domain, Layer: "package", Name: ext.importPath, ID: ext.slug,
			Properties:  map[string]any{"external": true},
			IfNotExists: true,
		}); err != nil {
			return err
		}
	}
	for i := range pkgs {
		for _, ri := range resolvedImports[i] {
			if err := enc.EdgeAdd(batch.EdgeAddArgs{
				Source:      domain + ":" + ri.fromSlug,
				Target:      domain + ":" + ri.toSlug,
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
