package graph_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/internal/graph"
)

func seedCarsDomain(t *testing.T, svc *graph.Service) {
	t.Helper()
	_, err := svc.AddDomain(t.Context(), graph.AddDomainInput{
		ID:     "cars",
		Layers: []string{"system", "subsystem", "part"},
	})
	require.NoError(t, err)
}

func TestAddNodeTopLayerHappyPath(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)

	n, err := svc.AddNode(t.Context(), graph.AddNodeInput{
		Domain: "cars",
		Layer:  "system",
		Name:   "Powertrain",
	})
	require.NoError(t, err)
	require.Equal(t, graph.NodeID("cars:powertrain"), n.ID)
	require.Nil(t, n.ParentID)
	require.Equal(t, int64(1), n.Revision)
}

func TestAddNodeExplicitIDOverridesDerivation(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)

	n, err := svc.AddNode(t.Context(), graph.AddNodeInput{
		Domain: "cars", Layer: "system", Name: "Powertrain", ID: "pt",
	})
	require.NoError(t, err)
	require.Equal(t, graph.NodeID("cars:pt"), n.ID)
}

func TestAddNodeRejectsExplicitInvalidSlug(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	_, err := svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "system", Name: "x", ID: "Bad ID"})
	require.ErrorIs(t, err, graph.ErrInvalidSlug)
}

func TestAddNodeDerivedSlugUnderivable(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	_, err := svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "system", Name: "!!!"})
	require.ErrorIs(t, err, graph.ErrSlugCannotDerive)
}

func TestAddNodeRejectsLayerNotInDomain(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	_, err := svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "chassis", Name: "x"})
	require.ErrorIs(t, err, graph.ErrLayerNotInDomain)
}

func TestAddNodeTopLayerWithParentRejected(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	_, err := svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "system", Name: "p", Parent: "cars:other"})
	require.ErrorIs(t, err, graph.ErrTopLayerCannotHaveParent)
}

func TestAddNodeNonTopRequiresParent(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	_, err := svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "subsystem", Name: "engine"})
	require.ErrorIs(t, err, graph.ErrParentLayerMismatch)
}

func TestAddNodeParentMustExist(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	_, err := svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "subsystem", Name: "engine", Parent: "cars:missing"})
	require.ErrorIs(t, err, graph.ErrNodeNotFound)
}

func TestAddNodeParentDomainMismatch(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	_, err := svc.AddDomain(t.Context(), graph.AddDomainInput{ID: "physics", Layers: []string{"law"}})
	require.NoError(t, err)
	_, err = svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "physics", Layer: "law", Name: "thermo"})
	require.NoError(t, err)
	_, err = svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "subsystem", Name: "engine", Parent: "physics:thermo"})
	require.ErrorIs(t, err, graph.ErrParentDomainMismatch)
}

func TestAddNodeParentLayerMismatch(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	_, err := svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "system", Name: "pt"})
	require.NoError(t, err)
	_, err = svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "part", Name: "piston", Parent: "cars:pt"})
	require.ErrorIs(t, err, graph.ErrParentLayerMismatch)
}

func TestAddNodeNonTopWithValidParent(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	_, err := svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "system", Name: "pt"})
	require.NoError(t, err)
	n, err := svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "subsystem", Name: "engine", Parent: "cars:pt"})
	require.NoError(t, err)
	require.NotNil(t, n.ParentID)
	require.Equal(t, graph.NodeID("cars:pt"), *n.ParentID)
}

func TestAddNodeAlreadyExists(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	in := graph.AddNodeInput{Domain: "cars", Layer: "system", Name: "pt"}
	_, err := svc.AddNode(t.Context(), in)
	require.NoError(t, err)
	_, err = svc.AddNode(t.Context(), in)
	require.ErrorIs(t, err, graph.ErrNodeAlreadyExists)
}

func TestAddNodeHasEmptyNamespacedProperties(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	n, err := svc.AddNode(t.Context(), graph.AddNodeInput{
		Domain: "cars", Layer: "system", Name: "pt",
	})
	require.NoError(t, err)
	require.NotNil(t, n.Properties)
}
