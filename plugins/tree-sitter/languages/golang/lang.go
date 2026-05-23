package golang

import (
	"context"
	"sync"

	sitter "github.com/smacker/go-tree-sitter"
	tsgolang "github.com/smacker/go-tree-sitter/golang"
)

type FileSource interface {
	Bytes() []byte
	RelPath() string
	PackageSlug() string
	FileSlug() string
}

type DeclSink interface {
	AddDecl(slug string, props map[string]any)
}

type ImportSink interface {
	AddImport(to string)
}

type CallSink interface {
	AddCall(fromDecl, toDecl string)
	HasDeclInFile(fileSlug, slug string) bool
}

type Decl struct {
	NameSlug   string
	Properties map[string]any
}

type Import struct {
	From, To string
}

type Call struct {
	FromDecl, ToDecl string
}

type GoLang struct {
	once   sync.Once
	parser *sitter.Parser
}

func New() *GoLang { return &GoLang{} }

func (g *GoLang) ID() string               { return "go" }
func (g *GoLang) FileExtensions() []string { return []string{".go"} }

func (g *GoLang) parse(ctx context.Context, src []byte) (*sitter.Tree, error) {
	g.once.Do(func() {
		g.parser = sitter.NewParser()
		g.parser.SetLanguage(tsgolang.GetLanguage())
	})
	return g.parser.ParseCtx(ctx, nil, src)
}

func (g *GoLang) ExtractFile(ctx context.Context, fs FileSource, ds DeclSink, is ImportSink) error {
	tree, err := g.parse(ctx, fs.Bytes())
	if err != nil {
		return err
	}
	defer tree.Close()
	root := tree.RootNode()
	walkDecls(root, fs.Bytes(), ds)
	walkImports(root, fs.Bytes(), is)
	return nil
}

func (g *GoLang) ResolvePackage(ctx context.Context, files []FileSource, cs CallSink) error {
	for _, f := range files {
		if err := walkCalls(g, f, cs); err != nil {
			return err
		}
	}
	return nil
}

