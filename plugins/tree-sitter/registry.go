package main

import "sync"

type Decl struct {
	NameSlug   string
	Properties map[string]any
}

type Import struct {
	From string
	To   string
}

type Call struct {
	FromDecl string
	ToDecl   string
}

type fileInfo struct {
	AbsPath      string
	RelPath      string
	BasenameSlug string
	PackagePath  string
	Source       []byte
	Decls        []Decl
}

type packageInfo struct {
	Path       string
	RelDir     string
	Slug       string
	Files      []fileInfo
	DeclByID   map[string]struct{}
	Imports    []Import
	Calls      []Call
	Properties map[string]any
}

type extractCtx struct {
	Domain   string
	Packages map[string]*packageInfo
}

type Language interface {
	ID() string
	FileExtensions() []string
	Extract(ctx *extractCtx, file fileInfo) error
	ResolveCalls(ctx *extractCtx, pkg *packageInfo) error
}

type registry struct {
	mu    sync.Mutex
	langs map[string]Language
}

func newRegistry() *registry { return &registry{langs: map[string]Language{}} }

func (r *registry) register(l Language) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.langs[l.ID()] = l
}

func (r *registry) lookup(id string) Language {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.langs[id]
}

func (r *registry) ids() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.langs))
	for id := range r.langs {
		out = append(out, id)
	}
	return out
}

var defaultRegistry = newRegistry()
