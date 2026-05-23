package main

import (
	"context"

	"github.com/ggfarmco/kg/plugins/tree-sitter/languages/golang"
)

type goAdapter struct{ g *golang.GoLang }

func (a *goAdapter) ID() string               { return a.g.ID() }
func (a *goAdapter) FileExtensions() []string { return a.g.FileExtensions() }

func (a *goAdapter) Extract(ec *extractCtx, f fileInfo) error {
	pkg := ec.Packages[f.PackagePath]
	if pkg == nil {
		return nil
	}
	fa := &fileAccess{f: &f, pkgSlug: pkg.Slug}
	da := &declAccess{pkg: pkg}
	ia := &importAccess{pkg: pkg, source: f.PackagePath}
	if err := a.g.ExtractFile(context.Background(), fa, da, ia); err != nil {
		return err
	}
	for i := range pkg.Files {
		if pkg.Files[i].RelPath == f.RelPath {
			pkg.Files[i].Decls = da.persist()
			break
		}
	}
	return nil
}

func (a *goAdapter) ResolveCalls(ec *extractCtx, p *packageInfo) error {
	srcs := make([]golang.FileSource, 0, len(p.Files))
	for i := range p.Files {
		srcs = append(srcs, &fileAccess{f: &p.Files[i], pkgSlug: p.Slug})
	}
	ca := &callAccess{pkg: p}
	return a.g.ResolvePackage(context.Background(), srcs, ca)
}

func init() {
	defaultRegistry.register(&goAdapter{g: golang.New()})
}

type fileAccess struct {
	f       *fileInfo
	pkgSlug string
}

func (a *fileAccess) Bytes() []byte       { return a.f.Source }
func (a *fileAccess) RelPath() string     { return a.f.RelPath }
func (a *fileAccess) PackageSlug() string { return a.pkgSlug }
func (a *fileAccess) FileSlug() string    { return a.f.BasenameSlug }

type declAccess struct {
	pkg   *packageInfo
	decls []golang.Decl
}

func (a *declAccess) AddDecl(slug string, props map[string]any) {
	a.decls = append(a.decls, golang.Decl{NameSlug: slug, Properties: props})
	a.pkg.DeclByID[slug] = struct{}{}
}

func (a *declAccess) persist() []Decl {
	out := make([]Decl, 0, len(a.decls))
	for _, d := range a.decls {
		out = append(out, Decl{NameSlug: d.NameSlug, Properties: d.Properties})
	}
	return out
}

type importAccess struct {
	pkg    *packageInfo
	source string
}

func (a *importAccess) AddImport(to string) {
	a.pkg.Imports = append(a.pkg.Imports, Import{From: a.source, To: to})
}

type callAccess struct{ pkg *packageInfo }

func (a *callAccess) AddCall(fromDecl, toDecl string) {
	a.pkg.Calls = append(a.pkg.Calls, Call{FromDecl: fromDecl, ToDecl: toDecl})
}

func (a *callAccess) HasDeclInFile(fileSlug, slug string) bool {
	for _, f := range a.pkg.Files {
		if f.BasenameSlug != fileSlug {
			continue
		}
		for _, d := range f.Decls {
			if d.NameSlug == slug {
				return true
			}
		}
	}
	return false
}
