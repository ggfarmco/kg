package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ggfarmco/kg/internal/graph"
	"github.com/ggfarmco/kg/snapshot"
)

type applyOpts struct {
	source              string
	domain              string
	scope               string
	dryRun              bool
	forceCascade        bool
	forceDomainTakeover bool
}

func newApplyCmd(c *cliCtx) *cobra.Command {
	opts := &applyOpts{}
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply a JSON snapshot (declarative diff+apply)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			snap, err := snapshot.Decode(os.Stdin)
			if err != nil {
				return err
			}
			if opts.source != "" && snap.Source != opts.source {
				return fmt.Errorf("SOURCE_MISMATCH: snapshot source=%q, --source=%q", snap.Source, opts.source)
			}
			if opts.domain != "" && snap.Domain != opts.domain {
				return fmt.Errorf("DOMAIN_MISMATCH: snapshot domain=%q, --domain=%q", snap.Domain, opts.domain)
			}
			aopts := graph.ApplyOptions{
				DryRun:              opts.dryRun,
				ForceCascade:        opts.forceCascade,
				ForceDomainTakeover: opts.forceDomainTakeover,
			}
			if opts.scope != "" {
				aopts.OverrideScope = snapshot.Scope(opts.scope)
			}
			res, err := svc.Apply(cmd.Context(), *snap, aopts)
			if err != nil {
				return err
			}
			return writeOK(c.stdout, res)
		},
	}
	cmd.Flags().StringVar(&opts.source, "source", "", "writer source id (must match snapshot.source)")
	cmd.Flags().StringVar(&opts.domain, "domain", "", "target domain (must match snapshot.domain)")
	cmd.Flags().StringVar(&opts.scope, "scope", "", "override snapshot.scope (domain-source|domain|additive)")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "compute diff, rollback")
	cmd.Flags().BoolVar(&opts.forceCascade, "force-cascade", false, "allow cleanup to drop nodes with children or foreign-claimed incident edges")
	cmd.Flags().BoolVar(&opts.forceDomainTakeover, "force-domain-takeover", false, "allow scope=domain even with foreign writers present")
	_ = cmd.MarkFlagRequired("source")
	_ = cmd.MarkFlagRequired("domain")
	return cmd
}
