package golang

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type callCapture struct{ calls [][2]string }

func (c *callCapture) AddCall(from, to string) {
	c.calls = append(c.calls, [2]string{from, to})
}

func (c *callCapture) HasDeclInFile(fileSlug, slug string) bool {
	return slug == "foo" || slug == "bar"
}

func TestResolveIntraPackageCalls(t *testing.T) {
	src := []byte(`package x

func foo() { bar() }
func bar() {}
func baz() { fmt.Println("hi") }
`)
	g := New()
	fs := &captureAll{src: src}
	cs := &callCapture{}
	require.NoError(t, g.ResolvePackage(context.Background(), []FileSource{fs}, cs))
	require.Len(t, cs.calls, 1)
	require.Equal(t, [2]string{"x/x-go::foo", "x/x-go::bar"}, cs.calls[0])
}
