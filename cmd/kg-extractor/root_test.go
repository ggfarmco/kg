package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRootHelpListsSubcommands(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := run([]string{"--help"}, &stdout, &stderr)
	require.Equal(t, 0, exit)
	out := stdout.String() + stderr.String()
	for _, sub := range []string{"list", "info", "extract"} {
		require.Contains(t, out, sub)
	}
}

func TestUnknownSubcommandReportsError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := run([]string{"bogus"}, &stdout, &stderr)
	require.NotEqual(t, 0, exit)
	require.True(t, strings.Contains(stdout.String(), "INVALID_INPUT") || strings.Contains(stderr.String(), "unknown command"))
}
