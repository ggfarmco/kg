package graph_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/internal/graph"
	"github.com/ggfarmco/kg/internal/graph/testutil"
)

func newService(t *testing.T) (*graph.Service, *testutil.FakeStore) {
	t.Helper()
	fs := testutil.NewFakeStore()
	clock := func() time.Time { return time.UnixMilli(1_700_000_000_000) }
	return graph.NewService(fs, clock), fs
}

func TestAddDomainHappyPath(t *testing.T) {
	svc, fs := newService(t)
	ctx := t.Context()

	d, err := svc.AddDomain(ctx, graph.AddDomainInput{
		ID:          "cars",
		Description: "vehicles",
		Layers:      []string{"system", "subsystem", "part"},
	})
	require.NoError(t, err)
	require.Equal(t, graph.DomainID("cars"), d.ID)
	require.Equal(t, int64(1), d.Revision)

	got, err := fs.GetDomain(ctx, "cars")
	require.NoError(t, err)
	require.Equal(t, []string{"system", "subsystem", "part"}, got.Layers)
}

func TestAddDomainRejectsInvalidID(t *testing.T) {
	svc, _ := newService(t)
	_, err := svc.AddDomain(t.Context(), graph.AddDomainInput{ID: "Cars", Layers: []string{"x"}})
	require.ErrorIs(t, err, graph.ErrInvalidSlug)
}

func TestAddDomainRejectsEmptyLayers(t *testing.T) {
	svc, _ := newService(t)
	_, err := svc.AddDomain(t.Context(), graph.AddDomainInput{ID: "cars", Layers: nil})
	require.Error(t, err)
}

func TestAddDomainAlreadyExists(t *testing.T) {
	svc, _ := newService(t)
	in := graph.AddDomainInput{ID: "cars", Layers: []string{"system"}}
	_, err := svc.AddDomain(t.Context(), in)
	require.NoError(t, err)
	_, err = svc.AddDomain(t.Context(), in)
	require.ErrorIs(t, err, graph.ErrDomainAlreadyExists)
}
