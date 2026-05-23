package batch_test

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ggfarmco/kg/batch"
)

func TestDecoderHappyPath(t *testing.T) {
	in := strings.NewReader(strings.Join([]string{
		`{"op":"meta","args":{"plugin":"x","total_ops":2}}`,
		`{"op":"domain.add","args":{"id":"a","layers":["l1"]}}`,
		`{"op":"node.add","args":{"domain":"a","layer":"l1","name":"n"}}`,
	}, "\n") + "\n")

	d := batch.NewDecoder(in)
	var ops []batch.Op
	for {
		op, err := d.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		ops = append(ops, op)
	}
	require.Len(t, ops, 3)
	require.Equal(t, batch.OpMeta, ops[0].Op)
	require.Equal(t, batch.OpDomainAdd, ops[1].Op)
	require.Equal(t, batch.OpNodeAdd, ops[2].Op)
}

func TestDecoderSkipsBlankLines(t *testing.T) {
	in := strings.NewReader("\n  \n" + `{"op":"meta","args":{}}` + "\n\n")
	d := batch.NewDecoder(in)
	op, err := d.Next()
	require.NoError(t, err)
	require.Equal(t, batch.OpMeta, op.Op)

	_, err = d.Next()
	require.ErrorIs(t, err, io.EOF)
}

func TestDecoderUnknownOpReportsLine(t *testing.T) {
	in := strings.NewReader(`{"op":"meta","args":{}}` + "\n" + `{"op":"foo.bar","args":{}}` + "\n")
	d := batch.NewDecoder(in)
	_, err := d.Next()
	require.NoError(t, err)
	_, err = d.Next()
	require.Error(t, err)
	var pe *batch.ParseError
	require.ErrorAs(t, err, &pe)
	require.Equal(t, 2, pe.Line)
	require.Contains(t, pe.Error(), "foo.bar")
}

func TestDecoderInvalidJSONReportsLine(t *testing.T) {
	in := strings.NewReader(`{"op":"meta","args":{}}` + "\n" + `not json` + "\n")
	d := batch.NewDecoder(in)
	_, err := d.Next()
	require.NoError(t, err)
	_, err = d.Next()
	require.Error(t, err)
	var pe *batch.ParseError
	require.ErrorAs(t, err, &pe)
	require.Equal(t, 2, pe.Line)
}

func TestEncoderRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	enc := batch.NewEncoder(&buf)
	require.NoError(t, enc.Meta(batch.MetaArgs{Plugin: "x", TotalOps: 1}))
	require.NoError(t, enc.DomainAdd(batch.DomainAddArgs{ID: "a", Layers: []string{"l"}, IfNotExists: true}))

	d := batch.NewDecoder(&buf)
	first, err := d.Next()
	require.NoError(t, err)
	require.Equal(t, batch.OpMeta, first.Op)
	second, err := d.Next()
	require.NoError(t, err)
	require.Equal(t, batch.OpDomainAdd, second.Op)
}

func TestEncoderEmitsOnePerLine(t *testing.T) {
	var buf bytes.Buffer
	enc := batch.NewEncoder(&buf)
	require.NoError(t, enc.NodeAdd(batch.NodeAddArgs{Domain: "a", Layer: "l", Name: "n"}))
	require.NoError(t, enc.NodeAdd(batch.NodeAddArgs{Domain: "a", Layer: "l", Name: "m"}))
	lines := bytes.Split(bytes.TrimRight(buf.Bytes(), "\n"), []byte("\n"))
	require.Len(t, lines, 2)
}
