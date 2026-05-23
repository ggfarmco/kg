package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegistryLookupAndRegister(t *testing.T) {
	r := newRegistry()
	require.Nil(t, r.lookup("nosuch"))
	r.register(&fakeLang{id: "fakelang"})
	got := r.lookup("fakelang")
	require.NotNil(t, got)
	require.Equal(t, "fakelang", got.ID())
}

type fakeLang struct{ id string }

func (f *fakeLang) ID() string                                           { return f.id }
func (f *fakeLang) FileExtensions() []string                             { return []string{".fake"} }
func (f *fakeLang) Extract(ctx *extractCtx, file fileInfo) error         { return nil }
func (f *fakeLang) ResolveCalls(ctx *extractCtx, pkg *packageInfo) error { return nil }
