package main

import "github.com/spf13/cobra"

func newInfoCmdReal(c *cliCtx) *cobra.Command {
	return &cobra.Command{Use: "info <name>", Short: "Show plugin info", RunE: func(*cobra.Command, []string) error { return nil }}
}
