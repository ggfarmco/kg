package golang

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type recordedDecl struct {
	slug  string
	props map[string]any
}

type captureAll struct {
	src     []byte
	decls   []recordedDecl
	imports [][2]string
}

func (c *captureAll) Bytes() []byte       { return c.src }
func (c *captureAll) RelPath() string     { return "x.go" }
func (c *captureAll) PackageSlug() string { return "x" }
func (c *captureAll) FileSlug() string    { return "x-go" }
func (c *captureAll) AddDecl(slug string, props map[string]any) {
	c.decls = append(c.decls, recordedDecl{slug, props})
}
func (c *captureAll) AddImport(to string) { c.imports = append(c.imports, [2]string{"", to}) }

func kindsOf(ds []recordedDecl) []string {
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		out = append(out, d.props["kind"].(string))
	}
	return out
}

func TestExtractFunctionDecls(t *testing.T) {
	src := []byte(`package x

func Foo(a string) (int, error) { return 0, nil }
func bar() {}
`)
	g := New()
	sinks := &captureAll{src: src}
	require.NoError(t, g.ExtractFile(context.Background(), sinks, sinks, sinks))
	require.Len(t, sinks.decls, 2)
	require.Equal(t, "foo", sinks.decls[0].slug)
	require.Equal(t, "function", sinks.decls[0].props["kind"])
	require.Equal(t, true, sinks.decls[0].props["exported"])
}

func TestExtractMethodDecls(t *testing.T) {
	src := []byte(`package x

type S struct{}

func (s *S) Hello() {}
`)
	g := New()
	sinks := &captureAll{src: src}
	require.NoError(t, g.ExtractFile(context.Background(), sinks, sinks, sinks))
	kinds := kindsOf(sinks.decls)
	require.Contains(t, kinds, "struct")
	require.Contains(t, kinds, "method")
	for _, d := range sinks.decls {
		if d.props["kind"] == "method" {
			require.Equal(t, "S", d.props["receiver"])
		}
	}
}

func TestExtractStructAndInterface(t *testing.T) {
	src := []byte(`package x

type Repo struct{ Name string }
type Reader interface { Read() error }
`)
	g := New()
	sinks := &captureAll{src: src}
	require.NoError(t, g.ExtractFile(context.Background(), sinks, sinks, sinks))
	require.Len(t, sinks.decls, 2)
	kinds := kindsOf(sinks.decls)
	require.ElementsMatch(t, []string{"struct", "interface"}, kinds)
}

func TestExtractVarAndConst(t *testing.T) {
	src := []byte(`package x

var pi = 3.14
const Greeting = "hi"
`)
	g := New()
	sinks := &captureAll{src: src}
	require.NoError(t, g.ExtractFile(context.Background(), sinks, sinks, sinks))
	require.Len(t, sinks.decls, 2)
}
