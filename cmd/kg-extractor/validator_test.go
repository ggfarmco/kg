package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateStreamHappyPath(t *testing.T) {
	in := strings.NewReader(strings.Join([]string{
		`{"op":"meta","args":{"plugin":"x"}}`,
		`{"op":"domain.add","args":{"id":"a","layers":["l"]}}`,
		`{"op":"node.add","args":{"domain":"a","layer":"l","name":"n"}}`,
	}, "\n") + "\n")

	var out bytes.Buffer
	require.NoError(t, validateStream(in, &out))
	require.Equal(t, 3, strings.Count(out.String(), "\n"))
}

func TestValidateStreamRejectsMissingArg(t *testing.T) {
	in := strings.NewReader(`{"op":"node.add","args":{"domain":"a","name":"n"}}` + "\n")
	var out bytes.Buffer
	err := validateStream(in, &out)
	require.Error(t, err)
	require.Contains(t, err.Error(), "layer")
}

func TestValidateStreamRejectsBadJSON(t *testing.T) {
	in := strings.NewReader("garbage\n")
	var out bytes.Buffer
	err := validateStream(in, &out)
	require.Error(t, err)
}

func TestValidateStreamEdgeAddV2WireAccepted(t *testing.T) {
	in := strings.NewReader(`{"op":"edge.add","args":{"src":"d:a","target":"d:b","type":"imports","source":"writer"}}` + "\n")
	var out bytes.Buffer
	require.NoError(t, validateStream(in, &out))
}

func TestValidateStreamEdgeAddV2WireNoWriterSourceAccepted(t *testing.T) {
	in := strings.NewReader(`{"op":"edge.add","args":{"src":"d:a","target":"d:b","type":"imports"}}` + "\n")
	var out bytes.Buffer
	require.NoError(t, validateStream(in, &out))
}

func TestValidateStreamEdgeAddMissingSrcRejected(t *testing.T) {
	in := strings.NewReader(`{"op":"edge.add","args":{"target":"d:b","type":"imports","source":"writer","writer_source":"writer"}}` + "\n")
	var out bytes.Buffer
	err := validateStream(in, &out)
	require.Error(t, err)
	require.Contains(t, err.Error(), "src")
}
