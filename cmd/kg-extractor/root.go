package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

type cliCtx struct {
	pluginsPath string
	stdout      io.Writer
	stderr      io.Writer
}

func newRootCmd(c *cliCtx) *cobra.Command {
	root := &cobra.Command{
		Use:           "kg-extractor",
		Short:         "kg-extractor — discover and dispatch kg extractor plugins",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.PersistentFlags().StringVar(&c.pluginsPath, "plugins-path", envOr("KG_EXTRACTOR_PLUGINS_PATH", defaultPluginsPath()), "colon-separated plugin discovery path")
	root.AddCommand(newListCmd(c), newInfoCmd(c), newExtractCmd(c))
	return root
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func defaultPluginsPath() string {
	if home, err := os.UserHomeDir(); err == nil {
		return fmt.Sprintf("%s/.config/kg-extractor/plugins", home)
	}
	return ""
}

func run(args []string, stdout, stderr io.Writer) int {
	c := &cliCtx{stdout: stdout, stderr: stderr}
	cmd := newRootCmd(c)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(args)
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		if errors.Is(err, errEnvelopeAlreadyWritten) {
			return 1
		}
		writeErr(stdout, "INVALID_INPUT", err.Error(), "")
		return 1
	}
	return 0
}

var errEnvelopeAlreadyWritten = errors.New("envelope already written")

func newListCmd(c *cliCtx) *cobra.Command    { return newListCmdReal(c) }
func newInfoCmd(c *cliCtx) *cobra.Command    { return newInfoCmdReal(c) }
func newExtractCmd(c *cliCtx) *cobra.Command { return newExtractCmdReal(c) }
