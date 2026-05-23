package graph_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/internal/graph"
)

func TestParseDomainID(t *testing.T) {
	got, err := graph.ParseDomainID("cars")
	require.NoError(t, err)
	require.Equal(t, graph.DomainID("cars"), got)

	_, err = graph.ParseDomainID("Cars")
	require.ErrorIs(t, err, graph.ErrInvalidSlug)
}
