package main

import (
	"context"
	"errors"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ggfarmco/kg/internal/graph"
)

func newDomainCmdReal(c *cliCtx) *cobra.Command {
	cmd := &cobra.Command{Use: "domain", Short: "Manage domains"}
	cmd.AddCommand(newDomainAddCmd(c), newDomainListCmd(c), newDomainGetCmd(c), newDomainDeleteCmd(c))
	return cmd
}

func newDomainAddCmd(c *cliCtx) *cobra.Command {
	var layers, description string
	var ifNotExists, dryRun bool
	cmd := &cobra.Command{
		Use:   "add <id>",
		Short: "Add a domain",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			in := graph.AddDomainInput{ID: args[0], Description: description, Layers: splitCSV(layers)}
			if dryRun {
				sentinel := errors.New("dry-run rollback")
				err := svc.InTx(cmd.Context(), func(ctx context.Context) error {
					if _, err := svc.AddDomain(ctx, in); err != nil {
						return err
					}
					return sentinel
				})
				if errors.Is(err, sentinel) {
					return writeOK(c.stdout, map[string]any{"dry_run": true})
				}
				return handleMaybeSkip(c.stdout, err, ifNotExists)
			}
			d, err := svc.AddDomain(cmd.Context(), in)
			if err != nil {
				return handleMaybeSkip(c.stdout, err, ifNotExists)
			}
			return writeOK(c.stdout, d)
		},
	}
	cmd.Flags().StringVar(&layers, "layers", "", "comma-separated ordered layer names (required)")
	cmd.Flags().StringVar(&description, "description", "", "free-form description")
	cmd.Flags().BoolVar(&ifNotExists, "if-not-exists", false, "skip with exit 0 if the domain already exists")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "validate without committing")
	_ = cmd.MarkFlagRequired("layers")
	return cmd
}

func newDomainListCmd(c *cliCtx) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List domains",
		RunE: func(cmd *cobra.Command, _ []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			ds, err := svc.ListDomains(cmd.Context())
			if err != nil {
				return err
			}
			return writeOK(c.stdout, ds)
		},
	}
}

func newDomainGetCmd(c *cliCtx) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Args:  cobra.ExactArgs(1),
		Short: "Get a domain by id",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			id, perr := graph.ParseDomainID(args[0])
			if perr != nil {
				return perr
			}
			d, err := svc.GetDomain(cmd.Context(), id)
			if err != nil {
				return err
			}
			return writeOK(c.stdout, d)
		},
	}
}

func newDomainDeleteCmd(c *cliCtx) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Args:  cobra.ExactArgs(1),
		Short: "Delete a domain",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			id, perr := graph.ParseDomainID(args[0])
			if perr != nil {
				return perr
			}
			if err := svc.DeleteDomain(cmd.Context(), id); err != nil {
				return err
			}
			return writeOK(c.stdout, map[string]any{"deleted": true, "id": id})
		},
	}
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}
