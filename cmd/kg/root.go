package main

import (
	"context"
	"errors"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/ggfarmco/kg/internal/graph"
	"github.com/ggfarmco/kg/internal/store"
)

type cliCtx struct {
	dbPath  string
	openSvc func(dbPath string) (*graph.Service, func(), error)
	stdout  io.Writer
	stderr  io.Writer
}

func newRootCmd(c *cliCtx) *cobra.Command {
	root := &cobra.Command{
		Use:           "kg",
		Short:         "kg — domain-agnostic knowledge graph engine",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.PersistentFlags().StringVar(&c.dbPath, "db", envOr("KG_DB", "./kg.db"), "path to the SQLite database file")
	root.AddCommand(newInitCmd(c), newDomainCmd(c), newNodeCmd(c), newEdgeCmd(c), newBatchCmd(c))
	return root
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func openService(dbPath string) (*graph.Service, func(), error) {
	st, err := store.Open(dbPath)
	if err != nil {
		return nil, nil, err
	}
	if err := st.UpsertSource(context.Background(), graph.Source{
		ID:        "manual",
		Trust:     100,
		FirstSeen: time.UnixMilli(0),
		LastSeen:  time.UnixMilli(0),
	}); err != nil {
		_ = st.Close()
		return nil, nil, err
	}
	svc := graph.NewService(st, nil)
	return svc, func() { _ = st.Close() }, nil
}

func run(args []string, stdout, stderr io.Writer) int {
	c := &cliCtx{openSvc: openService, stdout: stdout, stderr: stderr}
	cmd := newRootCmd(c)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(args)

	if wantsHelpJSON(args) {
		_ = writeOK(stdout, commandTree(cmd))
		return 0
	}

	err := cmd.ExecuteContext(context.Background())
	if err != nil {
		if errors.As(err, new(parseErrSentinel)) || errors.Is(err, errExitOne) {
			return 1
		}
		m := mapError(err)
		_ = writeErr(stdout, m.code, m.message, m.hint)
		return m.exit
	}
	return 0
}

func newInitCmd(c *cliCtx) *cobra.Command   { return newInitCmdReal(c) }
func newDomainCmd(c *cliCtx) *cobra.Command { return newDomainCmdReal(c) }
func newNodeCmd(c *cliCtx) *cobra.Command   { return newNodeCmdReal(c) }
func newEdgeCmd(c *cliCtx) *cobra.Command   { return newEdgeCmdReal(c) }
