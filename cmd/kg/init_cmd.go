package main

import "github.com/spf13/cobra"

func newInitCmdReal(c *cliCtx) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize the database (runs migrations)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			return writeOK(c.stdout, map[string]any{"initialized": true, "db": c.dbPath})
		},
	}
}
