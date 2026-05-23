package main

import "github.com/spf13/cobra"

func newExtractCmdReal(c *cliCtx) *cobra.Command {
	return &cobra.Command{Use: "extract", Short: "Run a plugin and emit ops", RunE: func(*cobra.Command, []string) error { return nil }}
}
