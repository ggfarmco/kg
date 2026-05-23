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

type batchOpts struct {
	continueOnError bool
	chunkSize       int
	dryRun          bool
	progress        bool
}

type batchEnvelope struct {
	Applied int  `json:"applied"`
	Skipped int  `json:"skipped"`
	Failed  int  `json:"failed,omitempty"`
	TookMs  int  `json:"took_ms"`
	DryRun  bool `json:"dry_run,omitempty"`
}

type batchFailure struct {
	Line    int    `json:"line"`
	Op      string `json:"op"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

var errExitOne = errors.New("batch partial failure")

func newBatchCmd(c *cliCtx) *cobra.Command {
	opts := &batchOpts{}
	cmd := &cobra.Command{
		Use:   "batch",
		Short: "Apply a JSONL stream of operations atomically",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if opts.continueOnError && opts.chunkSize > 0 {
				return errors.New("--continue-on-error and --chunk-size are mutually exclusive")
			}
			svc, closeFn, err := c.openSvc(c.dbPath)
			if err != nil {
				return err
			}
			defer closeFn()
			return runBatch(cmd.Context(), c.stdout, c.stderr, os.Stdin, svc, *opts)
		},
	}
	cmd.Flags().BoolVar(&opts.continueOnError, "continue-on-error", false, "keep applying ops on failure; final envelope lists failures")
	cmd.Flags().IntVar(&opts.chunkSize, "chunk-size", 0, "commit a transaction every N ops (0 = single transaction)")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "validate without committing")
	cmd.Flags().BoolVar(&opts.progress, "progress", false, "log progress to stderr roughly every 100ms")
	return cmd
}

func runBatch(ctx context.Context, stdout io.Writer, stderr io.Writer, stdin io.Reader, svc *graph.Service, opts batchOpts) error {
	ops, lines, total, parseErr := drainStream(stdin, stderr)
	if parseErr != nil {
		_ = writeErr(stdout, "INVALID_OP", parseErr.Error(), "")
		return parseErrSentinel{parseErr}
	}
	var prog *progressTicker
	if opts.progress {
		prog = newProgressTicker(stderr, total)
	}

	start := time.Now()
	switch {
	case opts.dryRun:
		return runDryRun(ctx, stdout, svc, ops, start)
	case opts.continueOnError:
		return runContinue(ctx, stdout, svc, ops, lines, start, prog)
	case opts.chunkSize > 0:
		return runChunked(ctx, stdout, svc, ops, opts.chunkSize, start, prog)
	default:
		return runAtomic(ctx, stdout, svc, ops, start, prog)
	}
}

type progressTicker struct {
	w     io.Writer
	total int64
	last  time.Time
}

func newProgressTicker(w io.Writer, total int64) *progressTicker {
	return &progressTicker{w: w, total: total, last: time.Now().Add(-time.Hour)}
}

func (p *progressTicker) tick(applied int) {
	if p == nil {
		return
	}
	now := time.Now()
	if now.Sub(p.last) < 100*time.Millisecond {
		return
	}
	p.last = now
	if p.total > 0 {
		fmt.Fprintf(p.w, "applied %d/%d\n", applied, p.total)
	} else {
		fmt.Fprintf(p.w, "applied %d\n", applied)
	}
}

func (p *progressTicker) flush(applied int) {
	if p == nil {
		return
	}
	if p.total > 0 {
		fmt.Fprintf(p.w, "applied %d/%d\n", applied, p.total)
	} else {
		fmt.Fprintf(p.w, "applied %d\n", applied)
	}
}

type dryRunResult struct {
	DryRun     bool `json:"dry_run"`
	WouldApply int  `json:"would_apply"`
	WouldSkip  int  `json:"would_skip"`
	TookMs     int  `json:"took_ms"`
}

func runDryRun(ctx context.Context, stdout io.Writer, svc *graph.Service, ops []batch.Op, start time.Time) error {
	var applied, skipped int
	sentinel := errors.New("dry-run rollback")
	err := svc.InTx(ctx, func(ctx context.Context) error {
		for _, op := range ops {
			a, err := applyOp(ctx, svc, op)
			if err != nil {
				return err
			}
			if a {
				applied++
			} else {
				skipped++
			}
		}
		return sentinel
	})
	if errors.Is(err, sentinel) {
		return writeOK(stdout, dryRunResult{
			DryRun:     true,
			WouldApply: applied,
			WouldSkip:  skipped,
			TookMs:     int(time.Since(start).Milliseconds()),
		})
	}
	return err
}

func runChunked(ctx context.Context, stdout io.Writer, svc *graph.Service, ops []batch.Op, chunkSize int, start time.Time, prog *progressTicker) error {
	env := batchEnvelope{}
	for i := 0; i < len(ops); i += chunkSize {
		end := i + chunkSize
		if end > len(ops) {
			end = len(ops)
		}
		chunk := ops[i:end]
		var chunkApplied, chunkSkipped int
		txErr := svc.InTx(ctx, func(ctx context.Context) error {
			chunkApplied, chunkSkipped = 0, 0
			for _, op := range chunk {
				applied, err := applyOp(ctx, svc, op)
				if err != nil {
					return err
				}
				if applied {
					chunkApplied++
					prog.tick(env.Applied + chunkApplied)
				} else {
					chunkSkipped++
				}
			}
			return nil
		})
		if txErr != nil {
			env.TookMs = int(time.Since(start).Milliseconds())
			return txErr
		}
		env.Applied += chunkApplied
		env.Skipped += chunkSkipped
	}
	prog.flush(env.Applied)
	env.TookMs = int(time.Since(start).Milliseconds())
	return writeOK(stdout, env)
}

func runAtomic(ctx context.Context, stdout io.Writer, svc *graph.Service, ops []batch.Op, start time.Time, prog *progressTicker) error {
	env := batchEnvelope{}
	txErr := svc.InTx(ctx, func(ctx context.Context) error {
		for _, op := range ops {
			applied, err := applyOp(ctx, svc, op)
			if err != nil {
				return err
			}
			if applied {
				env.Applied++
				prog.tick(env.Applied)
			} else {
				env.Skipped++
			}
		}
		return nil
	})
	if txErr != nil {
		return txErr
	}
	prog.flush(env.Applied)
	env.TookMs = int(time.Since(start).Milliseconds())
	return writeOK(stdout, env)
}

func runContinue(ctx context.Context, stdout io.Writer, svc *graph.Service, ops []batch.Op, lines []int, start time.Time, prog *progressTicker) error {
	env := batchEnvelope{}
	failures := []batchFailure{}
	for i, op := range ops {
		var applied bool
		err := svc.InTx(ctx, func(ctx context.Context) error {
			var inner error
			applied, inner = applyOp(ctx, svc, op)
			return inner
		})
		if err != nil {
			m := mapError(err)
			failures = append(failures, batchFailure{
				Line: lines[i], Op: string(op.Op), Code: m.code, Message: m.message,
			})
			env.Failed++
			continue
		}
		if applied {
			env.Applied++
			prog.tick(env.Applied)
		} else {
			env.Skipped++
		}
	}
	prog.flush(env.Applied)
	env.TookMs = int(time.Since(start).Milliseconds())
	if env.Failed == 0 {
		return writeOK(stdout, env)
	}
	return writeBatchPartial(stdout, env, failures)
}

func writeBatchPartial(w io.Writer, env batchEnvelope, failures []batchFailure) error {
	body := struct {
		OK       bool           `json:"ok"`
		Data     batchEnvelope  `json:"data"`
		Error    *envErr        `json:"error"`
		Failures []batchFailure `json:"failures"`
	}{
		OK:       false,
		Data:     env,
		Error:    &envErr{Code: "BATCH_PARTIAL", Message: fmt.Sprintf("%d ops failed; see failures[]", env.Failed)},
		Failures: failures,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(body); err != nil {
		return err
	}
	return errExitOne
}

type parseErrSentinel struct{ err error }

func (p parseErrSentinel) Error() string { return p.err.Error() }

func drainStream(r io.Reader, stderr io.Writer) (ops []batch.Op, lines []int, total int64, err error) {
	d := batch.NewDecoder(r)
	for {
		op, derr := d.Next()
		if errors.Is(derr, io.EOF) {
			return ops, lines, total, nil
		}
		if derr != nil {
			return nil, nil, 0, derr
		}
		if op.Op == batch.OpMeta {
			var m batch.MetaArgs
			if err := json.Unmarshal(op.Args, &m); err != nil {
				fmt.Fprintf(stderr, "meta: args parse error: %v\n", err)
			} else {
				fmt.Fprintf(stderr, "meta: plugin=%q total_ops=%d\n", m.Plugin, m.TotalOps)
				if m.TotalOps > 0 {
					total = m.TotalOps
				}
			}
			continue
		}
		ops = append(ops, op)
		lines = append(lines, d.Line())
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
