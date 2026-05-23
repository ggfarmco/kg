package main

import "github.com/spf13/cobra"

func newInfoCmdReal(c *cliCtx) *cobra.Command {
	return &cobra.Command{
		Use:   "info <name>",
		Args:  cobra.ExactArgs(1),
		Short: "Show plugin info",
		RunE: func(_ *cobra.Command, args []string) error {
			plugins, _ := discoverPlugins(c.pluginsPath)
			for _, p := range plugins {
				if p.Manifest.Name == args[0] {
					return writeOK(c.stdout, p.Manifest)
				}
			}
			writeErr(c.stdout, "PLUGIN_NOT_FOUND", "plugin "+args[0]+" not found", "run `kg-extractor list`")
			return errEnvelopeAlreadyWritten
		},
	}
}
