package main

import (
	"context"
	"io"
	"os"

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
	root.AddCommand(newInitCmd(c), newDomainCmd(c), newNodeCmd(c), newEdgeCmd(c))
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
	svc := graph.NewService(st, nil)
	return svc, func() { _ = st.Close() }, nil
}

func run(args []string, stdout, stderr io.Writer) int {
	c := &cliCtx{openSvc: openService, stdout: stdout, stderr: stderr}
	cmd := newRootCmd(c)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(context.Background())
	if err != nil {
		m := mapError(err)
		_ = writeErr(stdout, m.code, m.message, m.hint)
		return m.exit
	}
	return 0
}

func newInitCmd(*cliCtx) *cobra.Command {
	return &cobra.Command{Use: "init", Hidden: true, RunE: func(*cobra.Command, []string) error { return nil }}
}
func newDomainCmd(*cliCtx) *cobra.Command { return &cobra.Command{Use: "domain", Hidden: true} }
func newNodeCmd(*cliCtx) *cobra.Command   { return &cobra.Command{Use: "node", Hidden: true} }
func newEdgeCmd(*cliCtx) *cobra.Command   { return &cobra.Command{Use: "edge", Hidden: true} }
