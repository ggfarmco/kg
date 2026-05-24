package main

import (
	"bytes"
	"fmt"
	"io"

	"github.com/ggfarmco/kg/snapshot"
)

func validateSnapshot(r io.Reader, w io.Writer, expectedSource string) error {
	var raw bytes.Buffer
	if _, err := io.Copy(&raw, r); err != nil {
		return fmt.Errorf("read snapshot: %w", err)
	}
	snap, err := snapshot.Decode(bytes.NewReader(raw.Bytes()))
	if err != nil {
		return err
	}
	if err := snapshot.Validate(snap); err != nil {
		return err
	}
	if expectedSource != "" && snap.Source != expectedSource {
		return fmt.Errorf("SOURCE_MISMATCH: manifest source_id=%q, snapshot.source=%q", expectedSource, snap.Source)
	}
	_, err = w.Write(raw.Bytes())
	return err
}
