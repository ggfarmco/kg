package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
)

type runtimeKind string

const (
	runtimeNative              runtimeKind = "native"
	runtimeCommand             runtimeKind = "command"
	runtimeWASM                runtimeKind = "wasm"
	runtimeDeclarativeNative   runtimeKind = "declarative-native"
	runtimeDeclarativeCommand  runtimeKind = "declarative-command"
)

type manifest struct {
	Name               string      `json:"name"`
	Version            string      `json:"version"`
	Description        string      `json:"description"`
	Runtime            runtimeKind `json:"runtime"`
	Executable         string      `json:"executable,omitempty"`
	Command            []string    `json:"command,omitempty"`
	Module             string      `json:"module,omitempty"`
	SupportedLayers    []string    `json:"supported_layers,omitempty"`
	SupportedLanguages []string    `json:"supported_languages,omitempty"`
	SupportedScopes    []string    `json:"supported_scopes,omitempty"`
	SourceID           string      `json:"source_id,omitempty"`
}

var pluginNameRE = regexp.MustCompile(`^[a-z0-9-]+$`)

func parseManifest(path string) (*manifest, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var m manifest
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if !pluginNameRE.MatchString(m.Name) {
		return nil, fmt.Errorf("invalid plugin name %q (must match ^[a-z0-9-]+$)", m.Name)
	}
	if m.Version == "" {
		return nil, errors.New("manifest version is required")
	}
	if m.Description == "" {
		return nil, errors.New("manifest description is required")
	}
	switch m.Runtime {
	case runtimeNative, runtimeDeclarativeNative:
		if m.Executable == "" {
			return nil, fmt.Errorf("%s runtime requires executable", m.Runtime)
		}
	case runtimeCommand, runtimeDeclarativeCommand:
		if len(m.Command) == 0 {
			return nil, fmt.Errorf("%s runtime requires command[]", m.Runtime)
		}
	case runtimeWASM:
		if m.Module == "" {
			return nil, errors.New("wasm runtime requires module")
		}
	default:
		return nil, fmt.Errorf("unknown runtime %q", m.Runtime)
	}
	if m.SourceID == "" {
		m.SourceID = m.Name + ":" + m.Version
	}
	if len(m.SupportedScopes) == 0 {
		m.SupportedScopes = []string{"domain-source"}
	}
	return &m, nil
}

func (r runtimeKind) IsDeclarative() bool {
	return r == runtimeDeclarativeNative || r == runtimeDeclarativeCommand
}
