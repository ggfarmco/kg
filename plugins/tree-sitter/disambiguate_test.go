package main

import (
	"testing"
)

func TestDisambiguateDeclsByKind(t *testing.T) {
	decls := []Decl{
		{NameSlug: "listnodes", Properties: map[string]any{"kind": "const"}},
		{NameSlug: "listnodes", Properties: map[string]any{"kind": "method"}},
		{NameSlug: "store", Properties: map[string]any{"kind": "struct"}},
	}

	disambiguateDecls(decls)

	got := []string{decls[0].NameSlug, decls[1].NameSlug, decls[2].NameSlug}
	want := []string{"listnodes-const", "listnodes-method", "store"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("decls[%d].NameSlug = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestDisambiguateDeclsNumericFallback(t *testing.T) {
	decls := []Decl{
		{NameSlug: "intxorconn", Properties: map[string]any{"kind": "function"}},
		{NameSlug: "intxorconn", Properties: map[string]any{"kind": "function"}},
	}

	disambiguateDecls(decls)

	if decls[0].NameSlug == decls[1].NameSlug {
		t.Fatalf("same-kind collision not disambiguated: both %q", decls[0].NameSlug)
	}
	want := []string{"intxorconn-function", "intxorconn-function-2"}
	for i := range want {
		if decls[i].NameSlug != want[i] {
			t.Fatalf("decls[%d].NameSlug = %q, want %q", i, decls[i].NameSlug, want[i])
		}
	}
}
