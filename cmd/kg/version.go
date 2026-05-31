package main

import (
	"runtime/debug"

	"github.com/spf13/cobra"
)

var version = ""

func resolveVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}

func newVersionCmd(c *cliCtx) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "print the kg version",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return writeOK(c.stdout, map[string]string{"version": resolveVersion()})
		},
	}
}

func wantsVersion(args []string) bool {
	for _, a := range args {
		if a == "--version" {
			return true
		}
	}
	return false
}
