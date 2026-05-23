package main

import (
	"context"
	"errors"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/ggfarmco/kg/internal/graph"
)

func newEdgeCmdReal(c *cliCtx) *cobra.Command {
	cmd := &cobra.Command{Use: "edge", Short: "Manage edges"}
	cmd.AddCommand(newEdgeAddCmd(c), newEdgeListFromCmd(c), newEdgeListToCmd(c), newEdgeDeleteCmd(c))
	return cmd
}

func newEdgeAddCmd(c *cliCtx) *cobra.Command {
	var typ string
	var ifNotExists, dryRun bool
	cmd := &cobra.Command{
		Use:   "add <source-id> <target-id>",
		Args:  cobra.ExactArgs(2),
		Short: "Add an edge",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			in := graph.AddEdgeInput{Source: args[0], Target: args[1], Type: typ}
			if dryRun {
				sentinel := errors.New("dry-run rollback")
				err := svc.InTx(cmd.Context(), func(ctx context.Context) error {
					if _, err := svc.AddEdge(ctx, in); err != nil {
						return err
					}
					return sentinel
				})
				if errors.Is(err, sentinel) {
					return writeOK(c.stdout, map[string]any{"dry_run": true})
				}
				return handleMaybeSkip(c.stdout, err, ifNotExists)
			}
			e, err := svc.AddEdge(cmd.Context(), in)
			if err != nil {
				return handleMaybeSkip(c.stdout, err, ifNotExists)
			}
			return writeOK(c.stdout, e)
		},
	}
	cmd.Flags().StringVar(&typ, "type", "", "edge type (required)")
	cmd.Flags().BoolVar(&ifNotExists, "if-not-exists", false, "skip with exit 0 if the edge already exists")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "validate without committing")
	_ = cmd.MarkFlagRequired("type")
	return cmd
}

func newEdgeListFromCmd(c *cliCtx) *cobra.Command {
	var typ string
	cmd := &cobra.Command{
		Use:   "list-from <node-id>",
		Args:  cobra.ExactArgs(1),
		Short: "List edges originating at the given node",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			types := []string(nil)
			if typ != "" {
				types = []string{typ}
			}
			es, err := svc.EdgesFrom(cmd.Context(), graph.NodeID(args[0]), types)
			if err != nil {
				return err
			}
			return writeOK(c.stdout, es)
		},
	}
	cmd.Flags().StringVar(&typ, "type", "", "filter by edge type")
	return cmd
}

func newEdgeListToCmd(c *cliCtx) *cobra.Command {
	var typ string
	cmd := &cobra.Command{
		Use:   "list-to <node-id>",
		Args:  cobra.ExactArgs(1),
		Short: "List edges arriving at the given node",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			types := []string(nil)
			if typ != "" {
				types = []string{typ}
			}
			es, err := svc.EdgesTo(cmd.Context(), graph.NodeID(args[0]), types)
			if err != nil {
				return err
			}
			return writeOK(c.stdout, es)
		},
	}
	cmd.Flags().StringVar(&typ, "type", "", "filter by edge type")
	return cmd
}

func newEdgeDeleteCmd(c *cliCtx) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <edge-id>",
		Args:  cobra.ExactArgs(1),
		Short: "Delete an edge",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			n, perr := strconv.ParseInt(args[0], 10, 64)
			if perr != nil {
				return perr
			}
			if err := svc.DeleteEdge(cmd.Context(), graph.EdgeID(n)); err != nil {
				return err
			}
			return writeOK(c.stdout, map[string]any{"deleted": true, "id": n})
		},
	}
}
