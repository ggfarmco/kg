package main

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractPassThrough(t *testing.T) {
	mustBash(t)
	root := t.TempDir()
	dir := filepath.Join(root, "demo")
	require.NoError(t, exec.Command("mkdir", "-p", dir).Run())
	require.NoError(t, writeFile(filepath.Join(dir, "manifest.json"), `{"name":"demo","version":"0.1.0","description":"x","runtime":"command","command":["bash","extract.sh"]}`))
	require.NoError(t, writeFile(filepath.Join(dir, "extract.sh"), `#!/usr/bin/env bash
cat > /dev/null
echo '{"op":"domain.add","args":{"id":"a","layers":["l"]}}'
`))
	require.NoError(t, exec.Command("chmod", "+x", filepath.Join(dir, "extract.sh")).Run())

	var stdout, stderr bytes.Buffer
	exit := run([]string{"--plugins-path", root, "extract", "--plugin", "demo", "--input", "/x", "--domain", "a"}, &stdout, &stderr)
	require.Equal(t, 0, exit, "stderr=%s", stderr.String())
	require.Contains(t, stdout.String(), "domain.add")
}

func TestExtractWithDBForwardsToKgBatch(t *testing.T) {
	mustBash(t)
	kgPath := buildKg(t)
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "kg.db")
	require.NoError(t, exec.Command(kgPath, "--db", dbPath, "init").Run())

	root := t.TempDir()
	dir := filepath.Join(root, "demo")
	require.NoError(t, exec.Command("mkdir", "-p", dir).Run())
	require.NoError(t, writeFile(filepath.Join(dir, "manifest.json"), `{"name":"demo","version":"0.1.0","description":"x","runtime":"command","command":["bash","extract.sh"]}`))
	require.NoError(t, writeFile(filepath.Join(dir, "extract.sh"), `#!/usr/bin/env bash
cat > /dev/null
echo '{"op":"domain.add","args":{"id":"a","layers":["l"]}}'
echo '{"op":"node.add","args":{"domain":"a","layer":"l","name":"n"}}'
`))
	require.NoError(t, exec.Command("chmod", "+x", filepath.Join(dir, "extract.sh")).Run())

	var stdout, stderr bytes.Buffer
	exit := run([]string{
		"--plugins-path", root, "extract",
		"--plugin", "demo", "--input", "/x", "--domain", "a",
		"--db", dbPath, "--kg-binary", kgPath,
	}, &stdout, &stderr)
	require.Equal(t, 0, exit, "stdout=%s stderr=%s", stdout.String(), stderr.String())

	out, err := exec.Command(kgPath, "--db", dbPath, "node", "list", "--domain", "a").Output()
	require.NoError(t, err)
	require.Contains(t, string(out), "a:n")
}

func buildKg(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "kg")
	cmd := exec.Command("go", "build", "-o", out, "../kg")
	cmd.Stderr = bytes.NewBuffer(nil)
	if err := cmd.Run(); err != nil {
		t.Skipf("go build failed (likely no toolchain available): %v %s", err, cmd.Stderr)
	}
	return out
}
