package main

import (
	"sort"

	"github.com/ggfarmco/kg/snapshot"
)

const pluginSourceID = "tree-sitter:0.2.0"

func buildSnapshot(language, domain string, pkgs []*packageInfo, resolver *importResolver, includeExternalImports bool) snapshot.Snapshot {
	sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].Path < pkgs[j].Path })

	snap := snapshot.Snapshot{
		ProtocolVersion: snapshot.ProtocolVersion,
		Source:          pluginSourceID,
		Domain:          domain,
		Scope:           snapshot.ScopeDomainSource,
		DomainSpec: &snapshot.DomainSpec{
			ID: domain, Layers: []string{"package", "file", "decl"},
			Description: "Extracted by tree-sitter (" + language + ")",
		},
		Nodes: []snapshot.NodeSpec{},
		Edges: []snapshot.EdgeSpec{},
	}

	externalsByID := map[string]string{}
	var externalsOrder []string

	for _, p := range pkgs {
		pkgID := domain + ":" + p.Slug
		snap.Nodes = append(snap.Nodes, snapshot.NodeSpec{
			ID: pkgID, Layer: "package", Name: p.Path,
			Properties: nonNilMap(p.Properties),
		})
		for _, f := range p.Files {
			fileSlug := p.Slug + "/" + f.BasenameSlug
			fileID := domain + ":" + fileSlug
			fileProps := map[string]any{"rel_path": f.RelPath}
			if f.AbsPath != "" {
				fileProps["path"] = f.AbsPath
			}
			snap.Nodes = append(snap.Nodes, snapshot.NodeSpec{
				ID: fileID, Layer: "file", Parent: pkgID, Name: f.RelPath,
				Properties: fileProps,
			})
			for _, d := range f.Decls {
				declID := fileID + "::" + d.NameSlug
				snap.Nodes = append(snap.Nodes, snapshot.NodeSpec{
					ID: declID, Layer: "decl", Parent: fileID, Name: d.NameSlug,
					Properties: nonNilMap(d.Properties),
				})
			}
		}
	}

	for _, p := range pkgs {
		for _, imp := range p.Imports {
			toSlug, internal := resolver.Resolve(imp.To)
			if !internal {
				if !includeExternalImports {
					continue
				}
				toSlug = "ext-" + sanitizeSlug(imp.To)
				if _, seen := externalsByID[toSlug]; !seen {
					externalsByID[toSlug] = imp.To
					externalsOrder = append(externalsOrder, toSlug)
				}
			}
			snap.Edges = append(snap.Edges, snapshot.EdgeSpec{
				Src:    domain + ":" + sanitizeSlug(imp.From),
				Target: domain + ":" + toSlug,
				Type:   "imports",
			})
		}
	}
	for _, slug := range externalsOrder {
		snap.Nodes = append(snap.Nodes, snapshot.NodeSpec{
			ID: domain + ":" + slug, Layer: "package", Name: externalsByID[slug],
			Properties: map[string]any{"external": true},
		})
	}
	for _, p := range pkgs {
		for _, call := range p.Calls {
			snap.Edges = append(snap.Edges, snapshot.EdgeSpec{
				Src:    domain + ":" + call.FromDecl,
				Target: domain + ":" + call.ToDecl,
				Type:   "calls",
			})
		}
	}

	return snap
}

func nonNilMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	return m
}
