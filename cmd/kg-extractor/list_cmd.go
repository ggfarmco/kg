package main

import "github.com/spf13/cobra"

func newListCmdReal(c *cliCtx) *cobra.Command {
	return &cobra.Command{Use: "list", Short: "List discoverable plugins", RunE: func(*cobra.Command, []string) error { return nil }}
}
