package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/ggfarmco/kg/batch"
	"github.com/ggfarmco/kg/internal/graph"
)

type batchCounts struct {
	Applied int `json:"applied"`
	Skipped int `json:"skipped"`
	TookMs  int `json:"took_ms"`
}

func newBatchCmd(c *cliCtx) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "batch",
		Short: "Apply a JSONL stream of operations atomically",
		RunE: func(cmd *cobra.Command, _ []string) error {
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			return runBatch(cmd.Context(), c.stdout, c.stderr, os.Stdin, svc)
		},
	}
	return cmd
}

func runBatch(ctx context.Context, stdout io.Writer, stderr io.Writer, stdin io.Reader, svc *graph.Service) error {
	ops, parseErr := drainStream(stdin, stderr)
	if parseErr != nil {
		_ = writeErr(stdout, "INVALID_OP", parseErr.Error(), "")
		return parseErrSentinel{parseErr}
	}

	start := time.Now()
	counts := batchCounts{}
	txErr := svc.InTx(ctx, func(ctx context.Context) error {
		for _, op := range ops {
			applied, err := applyOp(ctx, svc, op)
			if err != nil {
				return err
			}
			if applied {
				counts.Applied++
			} else {
				counts.Skipped++
			}
		}
		return nil
	})
	if txErr != nil {
		return txErr
	}
	counts.TookMs = int(time.Since(start).Milliseconds())
	return writeOK(stdout, counts)
}

type parseErrSentinel struct{ err error }

func (p parseErrSentinel) Error() string { return p.err.Error() }

func drainStream(r io.Reader, stderr io.Writer) ([]batch.Op, error) {
	d := batch.NewDecoder(r)
	var ops []batch.Op
	for {
		op, err := d.Next()
		if errors.Is(err, io.EOF) {
			return ops, nil
		}
		if err != nil {
			return nil, err
		}
		if op.Op == batch.OpMeta {
			var m batch.MetaArgs
			if err := json.Unmarshal(op.Args, &m); err != nil {
				fmt.Fprintf(stderr, "meta: args parse error: %v\n", err)
			} else {
				fmt.Fprintf(stderr, "meta: plugin=%q total_ops=%d\n", m.Plugin, m.TotalOps)
			}
			continue
		}
		ops = append(ops, op)
	}
}

func applyOp(ctx context.Context, svc *graph.Service, op batch.Op) (applied bool, err error) {
	switch op.Op {
	case batch.OpDomainAdd:
		var a batch.DomainAddArgs
		if err := json.Unmarshal(op.Args, &a); err != nil {
			return false, fmt.Errorf("domain.add args: %w", err)
		}
		_, err := svc.AddDomain(ctx, graph.AddDomainInput{ID: a.ID, Description: a.Description, Layers: a.Layers})
		return classifyIfNotExists(err, a.IfNotExists, graph.ErrDomainAlreadyExists)

	case batch.OpNodeAdd:
		var a batch.NodeAddArgs
		if err := json.Unmarshal(op.Args, &a); err != nil {
			return false, fmt.Errorf("node.add args: %w", err)
		}
		_, err := svc.AddNode(ctx, graph.AddNodeInput{
			Domain: a.Domain, Layer: a.Layer, Name: a.Name,
			ID: a.ID, Parent: a.Parent, Summary: a.Summary, Properties: a.Properties,
		})
		return classifyIfNotExists(err, a.IfNotExists, graph.ErrNodeAlreadyExists)

	case batch.OpNodeUpdate:
		var a batch.NodeUpdateArgs
		if err := json.Unmarshal(op.Args, &a); err != nil {
			return false, fmt.Errorf("node.update args: %w", err)
		}
		_, err := svc.UpdateNode(ctx, graph.NodeID(a.ID), graph.UpdateNodeInput{Name: a.Name, Summary: a.Summary})
		if err != nil {
			return false, err
		}
		return true, nil

	case batch.OpNodeDelete:
		var a batch.NodeDeleteArgs
		if err := json.Unmarshal(op.Args, &a); err != nil {
			return false, fmt.Errorf("node.delete args: %w", err)
		}
		if err := svc.DeleteNode(ctx, graph.NodeID(a.ID)); err != nil {
			return false, err
		}
		return true, nil

	case batch.OpEdgeAdd:
		var a batch.EdgeAddArgs
		if err := json.Unmarshal(op.Args, &a); err != nil {
			return false, fmt.Errorf("edge.add args: %w", err)
		}
		_, err := svc.AddEdge(ctx, graph.AddEdgeInput{Source: a.Source, Target: a.Target, Type: a.Type, Properties: a.Properties})
		return classifyIfNotExists(err, a.IfNotExists, graph.ErrEdgeAlreadyExists)

	case batch.OpEdgeDelete:
		var a batch.EdgeDeleteArgs
		if err := json.Unmarshal(op.Args, &a); err != nil {
			return false, fmt.Errorf("edge.delete args: %w", err)
		}
		if err := svc.DeleteEdge(ctx, graph.EdgeID(a.ID)); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, fmt.Errorf("unknown op %q", op.Op)
}

func classifyIfNotExists(err error, ifNotExists bool, sentinel error) (bool, error) {
	if err == nil {
		return true, nil
	}
	if ifNotExists && errors.Is(err, sentinel) {
		return false, nil
	}
	return false, err
}
