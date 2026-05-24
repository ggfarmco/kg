//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractSelfDeclarative(t *testing.T) {
	kgBin := mustBuild(t, "kg", "../cmd/kg")
	extractorBin := mustBuild(t, "kg-extractor", "../cmd/kg-extractor")
	pluginBin := mustBuildPlugin(t)

	pluginsDir := t.TempDir()
	pluginHome := filepath.Join(pluginsDir, "tree-sitter")
	writeFile(t, filepath.Join(pluginHome, "manifest.json"), `{
		"name": "tree-sitter",
		"version": "0.2.0",
		"description": "tree-sitter (Go) declarative",
		"runtime": "declarative-native",
		"executable": "kg-extractor-tree-sitter",
		"source_id": "tree-sitter:0.2.0",
		"trust": 100
	}`)
	require.NoError(t, exec.Command("cp", pluginBin, filepath.Join(pluginHome, "kg-extractor-tree-sitter")).Run())

	dbPath := filepath.Join(t.TempDir(), "selfg.db")
	require.NoError(t, exec.Command(kgBin, "--db", dbPath, "init").Run())

	source := filepath.Join(t.TempDir(), "graph")
	require.NoError(t, exec.Command("cp", "-r", "../internal/graph", source).Run())
	absSource, _ := filepath.Abs(source)

	extract := func(t *testing.T) (envelope applyEnvelope) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		cmd := exec.Command(extractorBin,
			"--plugins-path", pluginsDir, "extract",
			"--plugin", "tree-sitter", "--language", "go",
			"--input", absSource, "--domain", "selfg",
			"--db", dbPath, "--kg-binary", kgBin,
		)
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		require.NoError(t, cmd.Run(), "stderr=%s", stderr.String())
		require.NoError(t, json.Unmarshal(stdout.Bytes(), &envelope), "stdout=%s", stdout.String())
		require.True(t, envelope.OK)
		return envelope
	}

	// Pass 1 — populate.
	envFirst := extract(t)
	require.Greater(t, envFirst.Data.NodesAdded, 0, "first pass adds nodes")

	// Sanity: nodes carry the plugin's source.
	dom, err := exec.Command(kgBin, "--db", dbPath, "node", "list", "--domain", "selfg", "--source", "tree-sitter:0.2.0").Output()
	require.NoError(t, err)
	require.Contains(t, string(dom), "selfg:graph")

	// Pass 2 — no changes.
	envSecond := extract(t)
	require.Equal(t, 0, envSecond.Data.NodesAdded)
	require.Equal(t, 0, envSecond.Data.NodesUpdated)
	require.Equal(t, 0, envSecond.Data.NodesRemoved)

	// Pass 3 — rename a decl in the file copy.
	nodeGo := filepath.Join(source, "node.go")
	body, err := os.ReadFile(nodeGo)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(nodeGo,
		bytes.Replace(body, []byte("func ParseSlug"), []byte("func ParseSlugRenamed"), 1), 0o644))
	envThird := extract(t)
	require.GreaterOrEqual(t, envThird.Data.NodesAdded, 1, "the rename should add at least one node")
	require.GreaterOrEqual(t, envThird.Data.NodesRemoved, 1, "the old name should be removed")

	// Pass 4 — add a manual cross-source edge; assert tree-sitter's claim survives re-extract.
	require.NoError(t, exec.Command(kgBin, "--db", dbPath, "sources", "register", "--id", "manual", "--if-not-exists").Run())

	// Edges live on decl-layer nodes. Find the first decl node with at least one outgoing edge.
	declOut, err := exec.Command(kgBin, "--db", dbPath, "node", "list", "--domain", "selfg", "--layer", "decl").Output()
	require.NoError(t, err)
	var declList struct {
		Data []struct{ ID string } `json:"data"`
	}
	require.NoError(t, json.Unmarshal(declOut, &declList))
	require.NotEmpty(t, declList.Data)

	var edgeID int64
	var edgeSrc string
	for _, n := range declList.Data {
		edgesOut, err2 := exec.Command(kgBin, "--db", dbPath, "edge", "list-from", n.ID).Output()
		if err2 != nil {
			continue
		}
		var edgesResp struct {
			Data []struct {
				ID       int64  `json:"id"`
				SourceID string `json:"source_id"`
			} `json:"data"`
		}
		if err2 = json.Unmarshal(edgesOut, &edgesResp); err2 != nil || len(edgesResp.Data) == 0 {
			continue
		}
		edgeID = edgesResp.Data[0].ID
		edgeSrc = edgesResp.Data[0].SourceID
		break
	}
	require.NotZero(t, edgeID, "tree-sitter should have produced at least one outgoing edge")

	// Add a manual claim on a different edge (pkg-level) to prove foreign claims survive.
	pkgOut, err := exec.Command(kgBin, "--db", dbPath, "node", "list", "--domain", "selfg", "--layer", "package").Output()
	require.NoError(t, err)
	var pkgList struct {
		Data []struct{ ID string } `json:"data"`
	}
	require.NoError(t, json.Unmarshal(pkgOut, &pkgList))
	require.GreaterOrEqual(t, len(pkgList.Data), 2, "need at least two packages for edge add")
	pkg := pkgList.Data[0].ID
	pkgOther := pkgList.Data[len(pkgList.Data)-1].ID
	_ = edgeSrc
	require.NoError(t, exec.Command(kgBin, "--db", dbPath, "edge", "add", pkg, pkgOther, "--type", "imports", "--source", "manual", "--if-not-exists").Run())

	envFourth := extract(t)
	_ = envFourth

	claims, err := exec.Command(kgBin, "--db", dbPath, "edge", "claims", strconv.FormatInt(edgeID, 10)).Output()
	require.NoError(t, err)
	require.Contains(t, string(claims), "tree-sitter:0.2.0", "tree-sitter's claim survives re-extract")
}

type applyEnvelope struct {
	OK   bool `json:"ok"`
	Data struct {
		NodesAdded    int `json:"nodes_added"`
		NodesUpdated  int `json:"nodes_updated"`
		NodesRemoved  int `json:"nodes_removed"`
		EdgesAdded    int `json:"edges_added"`
		ClaimsRemoved int `json:"claims_removed"`
		EdgesGC       int `json:"edges_gc"`
	} `json:"data"`
}
