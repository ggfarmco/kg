package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func execBatchCmd(t *testing.T, dbPath, stdin string, extraArgs ...string) (string, string, int) {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	_, err = w.WriteString(stdin)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	oldStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = oldStdin })

	var stdout, stderr bytes.Buffer
	args := append([]string{"--db", dbPath, "batch"}, extraArgs...)
	exit := run(args, &stdout, &stderr)
	return stdout.String(), stderr.String(), exit
}

func freshDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "kg.db")

	var stdout, stderr bytes.Buffer
	exit := run([]string{"--db", path, "init"}, &stdout, &stderr)
	require.Equal(t, 0, exit, "init failed: %s %s", stdout.String(), stderr.String())
	return path
}

func TestBatchHappyPath(t *testing.T) {
	db := freshDB(t)
	stream := strings.Join([]string{
		`{"op":"meta","args":{"plugin":"unit","total_ops":3}}`,
		`{"op":"domain.add","args":{"id":"a","layers":["l1","l2"]}}`,
		`{"op":"node.add","args":{"domain":"a","layer":"l1","name":"root"}}`,
		`{"op":"node.add","args":{"domain":"a","layer":"l2","name":"child","parent":"a:root"}}`,
	}, "\n") + "\n"

	stdout, stderr, exit := execBatchCmd(t, db, stream)
	require.Equal(t, 0, exit, "stderr=%s", stderr)

	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Applied int `json:"applied"`
			Skipped int `json:"skipped"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &env))
	require.True(t, env.OK)
	require.Equal(t, 3, env.Data.Applied)
	require.Equal(t, 0, env.Data.Skipped)
}

func TestBatchAtomicityRollsBackOnFailure(t *testing.T) {
	db := freshDB(t)
	stream := strings.Join([]string{
		`{"op":"domain.add","args":{"id":"a","layers":["l1"]}}`,
		`{"op":"node.add","args":{"domain":"a","layer":"l1","name":"ok"}}`,
		`{"op":"node.add","args":{"domain":"a","layer":"l1","name":"!!!"}}`,
	}, "\n") + "\n"

	_, _, exit := execBatchCmd(t, db, stream)
	require.NotEqual(t, 0, exit)

	var stdout, stderr bytes.Buffer
	listExit := run([]string{"--db", db, "domain", "list"}, &stdout, &stderr)
	require.Equal(t, 0, listExit)
	require.Contains(t, stdout.String(), `"data": []`, "the entire batch (including the leading domain.add) must roll back")
}

func TestBatchIfNotExistsCountsSkipped(t *testing.T) {
	db := freshDB(t)
	stream1 := `{"op":"domain.add","args":{"id":"a","layers":["l1"]}}` + "\n"
	_, _, exit := execBatchCmd(t, db, stream1)
	require.Equal(t, 0, exit)

	stream2 := `{"op":"domain.add","args":{"id":"a","layers":["l1"],"if_not_exists":true}}` + "\n"
	stdout, _, exit := execBatchCmd(t, db, stream2)
	require.Equal(t, 0, exit)
	require.Contains(t, stdout, `"applied": 0`)
	require.Contains(t, stdout, `"skipped": 1`)
}

func TestBatchInvalidJSONShortCircuits(t *testing.T) {
	db := freshDB(t)
	stream := strings.Join([]string{
		`{"op":"domain.add","args":{"id":"a","layers":["l1"]}}`,
		`not json`,
	}, "\n") + "\n"

	stdout, _, exit := execBatchCmd(t, db, stream)
	require.NotEqual(t, 0, exit)
	require.Contains(t, stdout, "INVALID_OP")

	var listOut, listErr bytes.Buffer
	listExit := run([]string{"--db", db, "domain", "list"}, &listOut, &listErr)
	require.Equal(t, 0, listExit)
	require.Contains(t, listOut.String(), `"data": []`)
}

func TestBatchContinueOnErrorReportsFailures(t *testing.T) {
	db := freshDB(t)
	stream := strings.Join([]string{
		`{"op":"domain.add","args":{"id":"a","layers":["l1"]}}`,
		`{"op":"node.add","args":{"domain":"a","layer":"l1","name":"good"}}`,
		`{"op":"node.add","args":{"domain":"a","layer":"l1","name":"!!!"}}`,
		`{"op":"node.add","args":{"domain":"a","layer":"l1","name":"alsoGood"}}`,
	}, "\n") + "\n"

	stdout, _, exit := execBatchCmd(t, db, stream, "--continue-on-error")
	require.NotEqual(t, 0, exit, "any failure causes nonzero exit")

	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Applied int `json:"applied"`
			Failed  int `json:"failed"`
		} `json:"data"`
		Failures []struct {
			Line int    `json:"line"`
			Op   string `json:"op"`
			Code string `json:"code"`
		} `json:"failures"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &env))
	require.False(t, env.OK)
	require.Equal(t, 3, env.Data.Applied, "good ops should commit")
	require.Equal(t, 1, env.Data.Failed)
	require.Len(t, env.Failures, 1)
	require.Equal(t, 3, env.Failures[0].Line, "the bad name is on the 3rd op (lines start at 1)")
}

func TestBatchContinueOnErrorAllSuccessReturnsOK(t *testing.T) {
	db := freshDB(t)
	stream := `{"op":"domain.add","args":{"id":"a","layers":["l1"]}}` + "\n"
	stdout, _, exit := execBatchCmd(t, db, stream, "--continue-on-error")
	require.Equal(t, 0, exit)
	require.Contains(t, stdout, `"ok": true`)
	require.NotContains(t, stdout, "failures")
}

func TestBatchChunkAndContinueMutuallyExclusive(t *testing.T) {
	db := freshDB(t)
	_, _, exit := execBatchCmd(t, db, "", "--continue-on-error", "--chunk-size", "10")
	require.NotEqual(t, 0, exit)
}
