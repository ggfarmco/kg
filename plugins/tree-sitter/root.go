package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

type cliCtx struct {
	stdout io.Writer
	stderr io.Writer
}

type stdinConfig struct {
	Input           string         `json:"input"`
	Domain          string         `json:"domain"`
	ProtocolVersion int            `json:"protocol_version"`
	Config          map[string]any `json:"config"`
}

func newRootCmd(c *cliCtx) *cobra.Command {
	var language string
	cmd := &cobra.Command{
		Use:           "kg-extractor-tree-sitter",
		Short:         "kg-extractor-tree-sitter — extract structure from source code via tree-sitter",
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := readStdinConfig(os.Stdin)
			if err != nil {
				return err
			}
			if cfg.ProtocolVersion != 1 {
				return fmt.Errorf("unsupported protocol_version %d", cfg.ProtocolVersion)
			}
			if language == "" {
				if v, ok := cfg.Config["language"].(string); ok {
					language = v
				}
			}
			if language == "" {
				return errors.New("--language not set and config.language missing")
			}
			lang := defaultRegistry.lookup(language)
			if lang == nil {
				return fmt.Errorf("LANGUAGE_NOT_SUPPORTED: %q (registered: %v)", language, defaultRegistry.ids())
			}
			return runExtraction(cmd.Context(), c.stdout, c.stderr, lang, cfg)
		},
	}
	cmd.Flags().StringVar(&language, "language", "", "language id (e.g. go); falls back to config.language")
	return cmd
}

func readStdinConfig(r io.Reader) (stdinConfig, error) {
	var cfg stdinConfig
	body, err := io.ReadAll(r)
	if err != nil {
		return cfg, fmt.Errorf("read stdin: %w", err)
	}
	if len(body) == 0 {
		return cfg, errors.New("empty stdin: kg-extractor must send the JSON config")
	}
	if err := json.Unmarshal(body, &cfg); err != nil {
		return cfg, fmt.Errorf("parse stdin config: %w", err)
	}
	return cfg, nil
}

func runExtraction(ctx context.Context, stdout io.Writer, stderr io.Writer, lang Language, cfg stdinConfig) error {
	return errors.New("runExtraction wired up in Phase 6")
}

func run(args []string, stdout, stderr io.Writer) int {
	c := &cliCtx{stdout: stdout, stderr: stderr}
	cmd := newRootCmd(c)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(args)
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
