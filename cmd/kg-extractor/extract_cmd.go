package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

type extractOpts struct {
	plugin     string
	input      string
	domain     string
	language   string
	configJSON string
	configFile string
	dbPath     string
	kgBinary   string
	quiet      bool
}

func newExtractCmdReal(c *cliCtx) *cobra.Command {
	opts := &extractOpts{}
	cmd := &cobra.Command{
		Use:   "extract",
		Short: "Run a plugin and forward its ops to kg batch (or stdout)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runExtract(cmd.Context(), c, opts)
		},
	}
	cmd.Flags().StringVar(&opts.plugin, "plugin", "", "plugin name (required)")
	cmd.Flags().StringVar(&opts.input, "input", "", "plugin-specific input path")
	cmd.Flags().StringVar(&opts.domain, "domain", "", "kg domain id")
	cmd.Flags().StringVar(&opts.language, "language", "", "language hint forwarded into config.language")
	cmd.Flags().StringVar(&opts.configJSON, "config-json", "", "inline JSON forwarded into plugin config")
	cmd.Flags().StringVar(&opts.configFile, "config-file", "", "path to JSON file forwarded into plugin config")
	cmd.Flags().StringVar(&opts.dbPath, "db", "", "if set, pipe ops to `kg --db <path> batch`")
	cmd.Flags().StringVar(&opts.kgBinary, "kg-binary", envOr("KG_BINARY", "kg"), "path to the kg binary (used when --db is set)")
	cmd.Flags().BoolVar(&opts.quiet, "quiet", false, "suppress plugin stderr forwarding")
	_ = cmd.MarkFlagRequired("plugin")
	return cmd
}

func runExtract(ctx context.Context, c *cliCtx, opts *extractOpts) error {
	plugins, _ := discoverPlugins(c.pluginsPath)
	var chosen *discoveredPlugin
	for i := range plugins {
		if plugins[i].Manifest.Name == opts.plugin {
			chosen = &plugins[i]
			break
		}
	}
	if chosen == nil {
		writeErr(c.stdout, "PLUGIN_NOT_FOUND", fmt.Sprintf("plugin %q not found", opts.plugin), "run `kg-extractor list`")
		return errEnvelopeAlreadyWritten
	}

	cfg := pluginConfig{Input: opts.input, Domain: opts.domain, ProtocolVersion: 1, Config: map[string]any{}}
	if opts.language != "" {
		cfg.Config["language"] = opts.language
	}
	if opts.configJSON != "" {
		var extra map[string]any
		if err := json.Unmarshal([]byte(opts.configJSON), &extra); err != nil {
			return fmt.Errorf("--config-json: %w", err)
		}
		for k, v := range extra {
			cfg.Config[k] = v
		}
	}
	if opts.configFile != "" {
		body, err := os.ReadFile(opts.configFile)
		if err != nil {
			return err
		}
		var extra map[string]any
		if err := json.Unmarshal(body, &extra); err != nil {
			return fmt.Errorf("--config-file %s: %w", opts.configFile, err)
		}
		for k, v := range extra {
			cfg.Config[k] = v
		}
	}

	var pluginStderr io.Writer = c.stderr
	if opts.quiet {
		pluginStderr = io.Discard
	}
	raw, err := invokePlugin(ctx, *chosen, cfg, pluginStderr)
	if err != nil {
		return err
	}

	var validated bytes.Buffer
	if err := validateStream(raw, &validated); err != nil {
		return err
	}

	if opts.dbPath == "" {
		_, err := c.stdout.Write(validated.Bytes())
		return err
	}
	return forwardToKgBatch(ctx, c, opts, validated.Bytes())
}

func forwardToKgBatch(ctx context.Context, c *cliCtx, opts *extractOpts, stream []byte) error {
	cmd := exec.CommandContext(ctx, opts.kgBinary, "--db", opts.dbPath, "batch")
	cmd.Stdin = bytes.NewReader(stream)
	cmd.Stdout = c.stdout
	cmd.Stderr = c.stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kg batch: %w", err)
	}
	return nil
}
