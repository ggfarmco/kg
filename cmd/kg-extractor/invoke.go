package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
)

type pluginConfig struct {
	Input           string         `json:"input,omitempty"`
	Domain          string         `json:"domain,omitempty"`
	ProtocolVersion int            `json:"protocol_version"`
	Config          map[string]any `json:"config,omitempty"`
}

func invokePlugin(ctx context.Context, p discoveredPlugin, cfg pluginConfig, stderr io.Writer) (*bytes.Buffer, error) {
	cmd, err := buildPluginCommand(ctx, p)
	if err != nil {
		return nil, err
	}

	configJSON, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	cmd.Stdin = bytes.NewReader(configJSON)
	cmd.Stderr = stderr

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("plugin %q failed: %w", p.Manifest.Name, err)
	}
	return &stdout, nil
}

func buildPluginCommand(ctx context.Context, p discoveredPlugin) (*exec.Cmd, error) {
	switch p.Manifest.Runtime {
	case runtimeNative:
		exe := p.Manifest.Executable
		if !filepath.IsAbs(exe) {
			exe = filepath.Join(p.Dir, exe)
		}
		return exec.CommandContext(ctx, exe), nil
	case runtimeCommand:
		if len(p.Manifest.Command) == 0 {
			return nil, errors.New("plugin command[] is empty")
		}
		cmd := exec.CommandContext(ctx, p.Manifest.Command[0], p.Manifest.Command[1:]...)
		cmd.Dir = p.Dir
		return cmd, nil
	case runtimeWASM:
		return nil, errors.New("WASM_NOT_SUPPORTED: wasm runtime is reserved for a future release")
	}
	return nil, fmt.Errorf("unknown runtime %q", p.Manifest.Runtime)
}
