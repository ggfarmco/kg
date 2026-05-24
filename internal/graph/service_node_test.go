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
		Source: "manual",
	})
	require.NoError(t, err)
}

func TestAddNodeTopLayerHappyPath(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)

	n, err := svc.AddNode(t.Context(), graph.AddNodeInput{
		Domain: "cars", Layer: "system", Name: "Powertrain", Source: "manual",
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
		Domain: "cars", Layer: "system", Name: "Powertrain", ID: "pt", Source: "manual",
	})
	require.NoError(t, err)
	require.Equal(t, graph.NodeID("cars:pt"), n.ID)
}

func TestAddNodeRejectsExplicitInvalidSlug(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	_, err := svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "system", Name: "x", ID: "Bad ID", Source: "manual"})
	require.ErrorIs(t, err, graph.ErrInvalidSlug)
}

func TestAddNodeDerivedSlugUnderivable(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	_, err := svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "system", Name: "!!!", Source: "manual"})
	require.ErrorIs(t, err, graph.ErrSlugCannotDerive)
}

func TestAddNodeRejectsLayerNotInDomain(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	_, err := svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "chassis", Name: "x", Source: "manual"})
	require.ErrorIs(t, err, graph.ErrLayerNotInDomain)
}

func TestAddNodeTopLayerWithParentRejected(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	_, err := svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "system", Name: "p", Parent: "cars:other", Source: "manual"})
	require.ErrorIs(t, err, graph.ErrTopLayerCannotHaveParent)
}

func TestAddNodeNonTopRequiresParent(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	_, err := svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "subsystem", Name: "engine", Source: "manual"})
	require.ErrorIs(t, err, graph.ErrParentLayerMismatch)
}

func TestAddNodeParentMustExist(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	_, err := svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "subsystem", Name: "engine", Parent: "cars:missing", Source: "manual"})
	require.ErrorIs(t, err, graph.ErrNodeNotFound)
}

func TestAddNodeParentDomainMismatch(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	_, err := svc.AddDomain(t.Context(), graph.AddDomainInput{ID: "physics", Layers: []string{"law"}, Source: "manual"})
	require.NoError(t, err)
	_, err = svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "physics", Layer: "law", Name: "thermo", Source: "manual"})
	require.NoError(t, err)
	_, err = svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "subsystem", Name: "engine", Parent: "physics:thermo", Source: "manual"})
	require.ErrorIs(t, err, graph.ErrParentDomainMismatch)
}

func TestAddNodeParentLayerMismatch(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	_, err := svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "system", Name: "pt", Source: "manual"})
	require.NoError(t, err)
	_, err = svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "part", Name: "piston", Parent: "cars:pt", Source: "manual"})
	require.ErrorIs(t, err, graph.ErrParentLayerMismatch)
}

func TestAddNodeNonTopWithValidParent(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	_, err := svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "system", Name: "pt", Source: "manual"})
	require.NoError(t, err)
	n, err := svc.AddNode(t.Context(), graph.AddNodeInput{Domain: "cars", Layer: "subsystem", Name: "engine", Parent: "cars:pt", Source: "manual"})
	require.NoError(t, err)
	require.NotNil(t, n.ParentID)
	require.Equal(t, graph.NodeID("cars:pt"), *n.ParentID)
}

func TestAddNodeAlreadyExists(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	in := graph.AddNodeInput{Domain: "cars", Layer: "system", Name: "pt", Source: "manual"}
	_, err := svc.AddNode(t.Context(), in)
	require.NoError(t, err)
	_, err = svc.AddNode(t.Context(), in)
	require.ErrorIs(t, err, graph.ErrNodeAlreadyExists)
}

func TestAddNodeHasEmptyNamespacedProperties(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	n, err := svc.AddNode(t.Context(), graph.AddNodeInput{
		Domain: "cars", Layer: "system", Name: "pt", Source: "manual",
	})
	require.NoError(t, err)
	require.NotNil(t, n.Properties)
}

func TestAddNodeAutoRegistersSourceAndStoresIt(t *testing.T) {
	svc, fs := newService(t)
	seedCarsDomain(t, svc)
	n, err := svc.AddNode(t.Context(), graph.AddNodeInput{
		Domain: "cars", Layer: "system", Name: "pt", Source: "tree-sitter:0.1.0",
	})
	require.NoError(t, err)
	require.Equal(t, graph.SourceID("tree-sitter:0.1.0"), n.Source)

	_, err = fs.GetSource(t.Context(), "tree-sitter:0.1.0")
	require.NoError(t, err)
}

func TestAddNodeRequiresSource(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	_, err := svc.AddNode(t.Context(), graph.AddNodeInput{
		Domain: "cars", Layer: "system", Name: "pt",
	})
	require.ErrorIs(t, err, graph.ErrSourceRequired)
}

func TestAddNodeSameIdDifferentSourceFails(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	_, err := svc.AddNode(t.Context(), graph.AddNodeInput{
		Domain: "cars", Layer: "system", Name: "pt", Source: "a",
	})
	require.NoError(t, err)
	_, err = svc.AddNode(t.Context(), graph.AddNodeInput{
		Domain: "cars", Layer: "system", Name: "pt", Source: "b",
	})
	require.ErrorIs(t, err, graph.ErrNodeOwnedByDifferentSource)
}

func TestUpdateNodeNameRequiresOwner(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	n, err := svc.AddNode(t.Context(), graph.AddNodeInput{
		Domain: "cars", Layer: "system", Name: "pt", Source: "a",
	})
	require.NoError(t, err)
	name := "new"
	_, err = svc.UpdateNode(t.Context(), n.ID, graph.UpdateNodeInput{Source: "b", Name: &name})
	require.ErrorIs(t, err, graph.ErrNodeNotOwner)
}

func TestDeleteNodeRequiresOwner(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	n, err := svc.AddNode(t.Context(), graph.AddNodeInput{
		Domain: "cars", Layer: "system", Name: "pt", Source: "a",
	})
	require.NoError(t, err)
	err = svc.DeleteNode(t.Context(), n.ID, "b")
	require.ErrorIs(t, err, graph.ErrNodeNotOwner)
}

func TestSetNodePropertiesReplacesOnlyOwnNamespace(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	n, err := svc.AddNode(t.Context(), graph.AddNodeInput{
		Domain: "cars", Layer: "system", Name: "pt", Source: "a",
		Properties: map[string]any{"x": float64(1)},
	})
	require.NoError(t, err)

	require.NoError(t, svc.SetNodeProperties(t.Context(), n.ID, "b", map[string]any{"y": float64(2)}))

	updated, err := svc.GetNode(t.Context(), n.ID)
	require.NoError(t, err)
	require.Equal(t, float64(1), updated.Properties["a"]["x"])
	require.Equal(t, float64(2), updated.Properties["b"]["y"])
}

func TestSetNodePropertiesReplaceWithinNamespace(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	n, _ := svc.AddNode(t.Context(), graph.AddNodeInput{
		Domain: "cars", Layer: "system", Name: "pt", Source: "a",
		Properties: map[string]any{"x": float64(1), "y": float64(2)},
	})
	require.NoError(t, svc.SetNodeProperties(t.Context(), n.ID, "a", map[string]any{"z": float64(3)}))
	updated, _ := svc.GetNode(t.Context(), n.ID)
	require.NotContains(t, updated.Properties["a"], "x")
	require.NotContains(t, updated.Properties["a"], "y")
	require.Equal(t, float64(3), updated.Properties["a"]["z"])
}

func TestDeleteNodePropertiesRemovesOnlyOwnNamespace(t *testing.T) {
	svc, _ := newService(t)
	seedCarsDomain(t, svc)
	n, _ := svc.AddNode(t.Context(), graph.AddNodeInput{
		Domain: "cars", Layer: "system", Name: "pt", Source: "a",
		Properties: map[string]any{"x": float64(1)},
	})
	require.NoError(t, svc.SetNodeProperties(t.Context(), n.ID, "b", map[string]any{"y": float64(2)}))
	require.NoError(t, svc.DeleteNodeProperties(t.Context(), n.ID, "a"))
	updated, _ := svc.GetNode(t.Context(), n.ID)
	require.NotContains(t, updated.Properties, graph.SourceID("a"))
	require.Equal(t, float64(2), updated.Properties["b"]["y"])
}
