package main

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/ggfarmco/kg/internal/graph"
)

func newSourcesCmd(c *cliCtx) *cobra.Command {
	cmd := &cobra.Command{Use: "sources", Short: "Manage source registry"}
	cmd.AddCommand(
		newSourcesListCmd(c), newSourcesShowCmd(c),
		newSourcesRegisterCmd(c), newSourcesUpdateCmd(c), newSourcesDeleteCmd(c),
	)
	return cmd
}

func newSourcesListCmd(c *cliCtx) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all sources",
		RunE: func(cmd *cobra.Command, _ []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			ss, err := svc.ListSources(cmd.Context())
			if err != nil {
				return err
			}
			return writeOK(c.stdout, ss)
		},
	}
}

func newSourcesShowCmd(c *cliCtx) *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Args:  cobra.ExactArgs(1),
		Short: "Show one source",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			s, err := svc.GetSource(cmd.Context(), graph.SourceID(args[0]))
			if err != nil {
				return err
			}
			return writeOK(c.stdout, s)
		},
	}
}

func newSourcesRegisterCmd(c *cliCtx) *cobra.Command {
	var id, description string
	var trust int
	var ifNotExists bool
	cmd := &cobra.Command{
		Use:   "register",
		Short: "Register a source",
		RunE: func(cmd *cobra.Command, _ []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			s, err := svc.RegisterSource(cmd.Context(), graph.SourceID(id), description, trust)
			if err != nil {
				if ifNotExists && errors.Is(err, graph.ErrSourceAlreadyExists) {
					return writeOK(c.stdout, map[string]any{"skipped": true, "reason": "already_exists"})
				}
				return err
			}
			return writeOK(c.stdout, s)
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "source id (required)")
	cmd.Flags().StringVar(&description, "description", "", "free-form description")
	cmd.Flags().IntVar(&trust, "trust", 100, "trust score (0-100)")
	cmd.Flags().BoolVar(&ifNotExists, "if-not-exists", false, "skip if the source already exists")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

func newSourcesUpdateCmd(c *cliCtx) *cobra.Command {
	var description string
	var trust int
	var trustSet bool
	cmd := &cobra.Command{
		Use:   "update <id>",
		Args:  cobra.ExactArgs(1),
		Short: "Update description and/or trust",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			cur, err := svc.GetSource(cmd.Context(), graph.SourceID(args[0]))
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("description") {
				cur.Description = description
			}
			if trustSet {
				cur.Trust = trust
			}
			if err := svc.UpdateSource(cmd.Context(), *cur); err != nil {
				return err
			}
			return writeOK(c.stdout, cur)
		},
	}
	cmd.Flags().StringVar(&description, "description", "", "new description")
	cmd.Flags().IntVar(&trust, "trust", 100, "new trust (0-100)")
	cmd.PreRun = func(cmd *cobra.Command, _ []string) {
		trustSet = cmd.Flags().Changed("trust")
	}
	return cmd
}

func newSourcesDeleteCmd(c *cliCtx) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Args:  cobra.ExactArgs(1),
		Short: "Delete a source (fails if it has owned entities)",
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			if err := svc.DeleteSource(cmd.Context(), graph.SourceID(args[0])); err != nil {
				return err
			}
			return writeOK(c.stdout, map[string]any{"deleted": true, "id": args[0]})
		},
	}
}
