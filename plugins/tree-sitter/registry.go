package main

import "sync"

type fileInfo struct {
	AbsPath      string
	RelPath      string
	BasenameSlug string
	PackagePath  string
	Source       []byte
}

type packageInfo struct {
	Path     string
	Slug     string
	Files    []fileInfo
	DeclByID map[string]struct{}
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
