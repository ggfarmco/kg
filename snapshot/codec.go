package snapshot

import (
	"encoding/json"
	"fmt"
	"io"
)

func Decode(r io.Reader) (*Snapshot, error) {
	dec := json.NewDecoder(r)
	var s Snapshot
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("decode snapshot: %w", err)
	}
	if dec.More() {
		return nil, fmt.Errorf("decode snapshot: trailing data (snapshot must be a single JSON document, not JSONL)")
	}
	return &s, nil
}

func Encode(w io.Writer, s Snapshot) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(s)
}
