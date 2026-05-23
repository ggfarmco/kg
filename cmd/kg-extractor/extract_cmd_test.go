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
