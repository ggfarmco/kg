package main

import (
	"context"
	"errors"

	"github.com/spf13/cobra"

	"github.com/ggfarmco/kg/internal/graph"
)

func newNodeCmdReal(c *cliCtx) *cobra.Command {
	cmd := &cobra.Command{Use: "node", Short: "Manage nodes"}
	cmd.AddCommand(
		newNodeAddCmd(c), newNodeGetCmd(c), newNodeListCmd(c),
		newNodeChildrenCmd(c), newNodeUpdateCmd(c), newNodeDeleteCmd(c),
	)
	return cmd
}

func newNodeAddCmd(c *cliCtx) *cobra.Command {
	var domain, layer, name, id, parent, source, summary string
	var ifNotExists, dryRun bool
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a node",
		RunE: func(cmd *cobra.Command, _ []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			in := graph.AddNodeInput{Domain: domain, Layer: layer, Name: name, ID: id, Parent: parent, Source: source}
			if dryRun {
				sentinel := errors.New("dry-run rollback")
				err := svc.InTx(cmd.Context(), func(ctx context.Context) error {
					if _, err := svc.AddNode(ctx, in); err != nil {
						return err
					}
					return sentinel
				})
				if errors.Is(err, sentinel) {
					return writeOK(c.stdout, map[string]any{"dry_run": true})
				}
				return handleMaybeSkip(c.stdout, err, ifNotExists)
			}
			n, err := svc.AddNode(cmd.Context(), in)
			if err != nil {
				return handleMaybeSkip(c.stdout, err, ifNotExists)
			}
			return writeOK(c.stdout, n)
		},
	}
	cmd.Flags().StringVar(&domain, "domain", "", "domain id (required)")
	cmd.Flags().StringVar(&layer, "layer", "", "layer name (required)")
	cmd.Flags().StringVar(&name, "name", "", "human-readable name (required)")
	cmd.Flags().StringVar(&id, "id", "", "explicit slug; if omitted, derived from name")
	cmd.Flags().StringVar(&parent, "parent", "", "parent node id (required unless top layer)")
	cmd.Flags().StringVar(&source, "source", "cli", "writer source id")
	cmd.Flags().StringVar(&summary, "summary", "", "optional summary text")
	cmd.Flags().BoolVar(&ifNotExists, "if-not-exists", false, "skip with exit 0 if the node already exists")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "validate without committing")
	for _, f := range []string{"domain", "layer", "name"} {
		_ = cmd.MarkFlagRequired(f)
	}
	return cmd
}

func newNodeGetCmd(c *cliCtx) *cobra.Command {
	return &cobra.Command{
		Use:   "get <node-id>",
		Args:  cobra.ExactArgs(1),
		Short: "Get a node by id",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			n, err := svc.GetNode(cmd.Context(), graph.NodeID(args[0]))
			if err != nil {
				return err
			}
			return writeOK(c.stdout, n)
		},
	}
}

func newNodeListCmd(c *cliCtx) *cobra.Command {
	var domain, layer string
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List nodes",
		RunE: func(cmd *cobra.Command, _ []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			ns, err := svc.ListNodes(cmd.Context(), graph.NodeFilter{
				Domain: graph.DomainID(domain), Layer: layer, Limit: limit,
			})
			if err != nil {
				return err
			}
			return writeOK(c.stdout, ns)
		},
	}
	cmd.Flags().StringVar(&domain, "domain", "", "filter by domain id")
	cmd.Flags().StringVar(&layer, "layer", "", "filter by layer name")
	cmd.Flags().IntVar(&limit, "limit", 0, "max rows (0 = unlimited)")
	return cmd
}

func newNodeChildrenCmd(c *cliCtx) *cobra.Command {
	return &cobra.Command{
		Use:   "children <node-id>",
		Args:  cobra.ExactArgs(1),
		Short: "List direct children of a node",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			ns, err := svc.ChildrenOf(cmd.Context(), graph.NodeID(args[0]))
			if err != nil {
				return err
			}
			return writeOK(c.stdout, ns)
		},
	}
}

func newNodeUpdateCmd(c *cliCtx) *cobra.Command {
	var name, source, summary string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "update <node-id>",
		Args:  cobra.ExactArgs(1),
		Short: "Update a node's name or summary",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			in := graph.UpdateNodeInput{Source: graph.SourceID(source)}
			if cmd.Flags().Changed("name") {
				in.Name = &name
			}
			_ = summary
			if dryRun {
				sentinel := errors.New("dry-run rollback")
				err := svc.InTx(cmd.Context(), func(ctx context.Context) error {
					if _, err := svc.UpdateNode(ctx, graph.NodeID(args[0]), in); err != nil {
						return err
					}
					return sentinel
				})
				if errors.Is(err, sentinel) {
					return writeOK(c.stdout, map[string]any{"dry_run": true})
				}
				return err
			}
			n, err := svc.UpdateNode(cmd.Context(), graph.NodeID(args[0]), in)
			if err != nil {
				return err
			}
			return writeOK(c.stdout, n)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "new name")
	cmd.Flags().StringVar(&source, "source", "cli", "writer source id")
	cmd.Flags().StringVar(&summary, "summary", "", "new summary")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "validate without committing")
	return cmd
}

func newNodeDeleteCmd(c *cliCtx) *cobra.Command {
	var source string
	cmd := &cobra.Command{
		Use:   "delete <node-id>",
		Args:  cobra.ExactArgs(1),
		Short: "Delete a node",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			id := graph.NodeID(args[0])
			if err := svc.DeleteNode(cmd.Context(), id, graph.SourceID(source)); err != nil {
				return err
			}
			return writeOK(c.stdout, map[string]any{"deleted": true, "id": id})
		},
	}
	cmd.Flags().StringVar(&source, "source", "cli", "writer source id")
	return cmd
}
