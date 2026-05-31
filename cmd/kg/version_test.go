package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestVersionFlagEmitsEnvelope(t *testing.T) {
	version = "v9.9.9"
	t.Cleanup(func() { version = "" })

	var out bytes.Buffer
	if code := run([]string{"--version"}, &out, &out); code != 0 {
		t.Fatalf("run(--version) exit = %d, want 0", code)
	}

	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Version string `json:"version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("output is not the JSON envelope: %v\n%s", err, out.String())
	}
	if !env.OK || env.Data.Version != "v9.9.9" {
		t.Fatalf("got ok=%v version=%q, want ok=true version=v9.9.9", env.OK, env.Data.Version)
	}
}

func TestResolveVersionFallsBackToDev(t *testing.T) {
	version = ""
	t.Cleanup(func() { version = "" })

	if got := resolveVersion(); got == "" {
		t.Fatal("resolveVersion() returned empty string, want non-empty fallback")
	}
}
