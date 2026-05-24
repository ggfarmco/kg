package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ggfarmco/kg/internal/graph"
	"github.com/ggfarmco/kg/snapshot"
)

func newExportCmd(c *cliCtx) *cobra.Command {
	var domain, source, format string
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export current (domain, source) state as a snapshot JSON document",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if format != "snapshot" {
				return fmt.Errorf("--format must be 'snapshot' (got %q)", format)
			}
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()

			dID := graph.DomainID(domain)
			srcID := graph.SourceID(source)

			d, err := svc.GetDomain(cmd.Context(), dID)
			if err != nil {
				return err
			}

			nodes, err := svc.ListNodes(cmd.Context(), graph.NodeFilter{Domain: dID, Source: srcID})
			if err != nil {
				return err
			}

			snap := snapshot.Snapshot{
				ProtocolVersion: snapshot.ProtocolVersion,
				Source:          source,
				Domain:          domain,
				Scope:           snapshot.ScopeDomainSource,
				DomainSpec: &snapshot.DomainSpec{
					ID: string(d.ID), Layers: d.Layers, Description: d.Description,
				},
				Nodes: make([]snapshot.NodeSpec, 0, len(nodes)),
				Edges: []snapshot.EdgeSpec{},
			}
			for _, n := range nodes {
				spec := snapshot.NodeSpec{
					ID: string(n.ID), Layer: n.Layer, Name: n.Name,
					Properties: n.Properties[srcID],
				}
				if n.ParentID != nil {
					spec.Parent = string(*n.ParentID)
				}
				snap.Nodes = append(snap.Nodes, spec)
			}

			claimedIDs, err := svc.EdgeIDsClaimedBy(cmd.Context(), srcID)
			if err != nil {
				return err
			}
			for _, eid := range claimedIDs {
				e, err := svc.GetEdge(cmd.Context(), eid)
				if err != nil {
					return err
				}
				snap.Edges = append(snap.Edges, snapshot.EdgeSpec{
					Src: string(e.SourceID), Target: string(e.TargetID),
					Type: e.Type, Properties: e.Properties[srcID],
				})
			}

			return snapshot.Encode(c.stdout, snap)
		},
	}
	cmd.Flags().StringVar(&domain, "domain", "", "domain id (required)")
	cmd.Flags().StringVar(&source, "source", "", "writer source id (required)")
	cmd.Flags().StringVar(&format, "format", "snapshot", "output format (only 'snapshot' supported in v3)")
	_ = cmd.MarkFlagRequired("domain")
	_ = cmd.MarkFlagRequired("source")
	return cmd
}
