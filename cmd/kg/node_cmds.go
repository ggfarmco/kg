package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

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
	var domain, layer, name, id, parent, source, summary, propertiesJSON string
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
			if propertiesJSON != "" {
				if err := json.Unmarshal([]byte(propertiesJSON), &in.Properties); err != nil {
					return fmt.Errorf("--properties: %w", err)
				}
			}
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
	cmd.Flags().StringVar(&propertiesJSON, "properties", "", "JSON object of properties for this source's namespace")
	cmd.Flags().BoolVar(&ifNotExists, "if-not-exists", false, "skip with exit 0 if the node already exists")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "validate without committing")
	for _, f := range []string{"domain", "layer", "name"} {
		_ = cmd.MarkFlagRequired(f)
	}
	return cmd
}

func newNodeGetCmd(c *cliCtx) *cobra.Command {
	var source string
	var merged bool
	cmd := &cobra.Command{
		Use:   "get <node-id>",
		Args:  cobra.ExactArgs(1),
		Short: "Get a node (default: raw namespaced properties)",
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
			switch {
			case source != "":
				return writeOK(c.stdout, nodeFlattenedView(*n, graph.SourceID(source)))
			case merged:
				return writeOK(c.stdout, nodeMergedView(*n))
			}
			return writeOK(c.stdout, n)
		},
	}
	cmd.Flags().StringVar(&source, "source", "", "show only this source's namespace (flattened)")
	cmd.Flags().BoolVar(&merged, "merged", false, "union of all namespaces with _property_sources attribution")
	return cmd
}

func newNodeListCmd(c *cliCtx) *cobra.Command {
	var domain, layer, source string
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
				Domain: graph.DomainID(domain), Layer: layer, Source: graph.SourceID(source), Limit: limit,
			})
			if err != nil {
				return err
			}
			return writeOK(c.stdout, ns)
		},
	}
	cmd.Flags().StringVar(&domain, "domain", "", "filter by domain id")
	cmd.Flags().StringVar(&layer, "layer", "", "filter by layer name")
	cmd.Flags().StringVar(&source, "source", "", "filter by owning source id")
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
	var name, source, summary, propertiesJSON string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "update <node-id>",
		Args:  cobra.ExactArgs(1),
		Short: "Update a node's name (owner only) or properties (any writer, within own namespace)",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			id := graph.NodeID(args[0])
			_ = summary
			if dryRun {
				sentinel := errors.New("dry-run rollback")
				err := svc.InTx(cmd.Context(), func(ctx context.Context) error {
					in := graph.UpdateNodeInput{Source: graph.SourceID(source)}
					if cmd.Flags().Changed("name") {
						in.Name = &name
					}
					if _, err := svc.UpdateNode(ctx, id, in); err != nil {
						return err
					}
					return sentinel
				})
				if errors.Is(err, sentinel) {
					return writeOK(c.stdout, map[string]any{"dry_run": true})
				}
				return err
			}
			if cmd.Flags().Changed("name") {
				if _, err := svc.UpdateNode(cmd.Context(), id, graph.UpdateNodeInput{
					Source: graph.SourceID(source), Name: &name,
				}); err != nil {
					return err
				}
			}
			if propertiesJSON != "" {
				var props map[string]any
				if err := json.Unmarshal([]byte(propertiesJSON), &props); err != nil {
					return fmt.Errorf("--properties: %w", err)
				}
				if err := svc.SetNodeProperties(cmd.Context(), id, graph.SourceID(source), props); err != nil {
					return err
				}
			}
			n, err := svc.GetNode(cmd.Context(), id)
			if err != nil {
				return err
			}
			return writeOK(c.stdout, n)
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "new name")
	cmd.Flags().StringVar(&source, "source", "cli", "writer source id (default cli)")
	cmd.Flags().StringVar(&summary, "summary", "", "new summary")
	cmd.Flags().StringVar(&propertiesJSON, "properties", "", "JSON object of properties for this source's namespace")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "validate without committing")
	return cmd
}

func newNodeDeleteCmd(c *cliCtx) *cobra.Command {
	var source string
	var forceCascade bool
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
			if forceCascade {
				if err := svc.ForceDeleteNode(cmd.Context(), id); err != nil {
					return err
				}
			} else {
				if err := svc.DeleteNode(cmd.Context(), id, graph.SourceID(source)); err != nil {
					return err
				}
			}
			return writeOK(c.stdout, map[string]any{"deleted": true, "id": id})
		},
	}
	cmd.Flags().StringVar(&source, "source", "cli", "writer source id")
	cmd.Flags().BoolVar(&forceCascade, "force-cascade", false, "drop the node ignoring foreign claims and children")
	return cmd
}

func nodeFlattenedView(n graph.Node, source graph.SourceID) map[string]any {
	out := map[string]any{
		"id":         n.ID,
		"domain":     n.Domain,
		"layer":      n.Layer,
		"name":       n.Name,
		"parent_id":  n.ParentID,
		"source":     n.Source,
		"revision":   n.Revision,
		"created_at": n.CreatedAt,
		"updated_at": n.UpdatedAt,
	}
	for k, v := range n.Properties[source] {
		out[k] = v
	}
	return out
}

func nodeMergedView(n graph.Node) map[string]any {
	type contrib struct {
		source graph.SourceID
		value  any
	}
	keys := map[string]contrib{}
	for src, m := range n.Properties {
		for k, v := range m {
			c, ok := keys[k]
			if !ok || src < c.source {
				keys[k] = contrib{source: src, value: v}
			}
		}
	}
	props := map[string]any{}
	srcs := map[string]string{}
	for k, c := range keys {
		props[k] = c.value
		srcs[k] = string(c.source)
	}
	return map[string]any{
		"id":                n.ID,
		"domain":            n.Domain,
		"layer":             n.Layer,
		"name":              n.Name,
		"parent_id":         n.ParentID,
		"source":            n.Source,
		"properties":        props,
		"_property_sources": srcs,
		"revision":          n.Revision,
		"created_at":        n.CreatedAt,
		"updated_at":        n.UpdatedAt,
	}
}
