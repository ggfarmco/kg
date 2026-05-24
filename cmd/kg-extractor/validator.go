package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/ggfarmco/kg/batch"
)

func validateStream(in io.Reader, out io.Writer) error {
	d := batch.NewDecoder(in)
	enc := batch.NewEncoder(out)
	for {
		op, err := d.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := validateOp(op, d.Line()); err != nil {
			return err
		}
		switch op.Op {
		case batch.OpMeta:
			var a batch.MetaArgs
			_ = json.Unmarshal(op.Args, &a)
			if err := enc.Meta(a); err != nil {
				return err
			}
		default:
			if _, werr := out.Write(append(rawOpLine(op), '\n')); werr != nil {
				return werr
			}
		}
	}
}

func rawOpLine(op batch.Op) []byte {
	b, _ := json.Marshal(op)
	return b
}

func validateOp(op batch.Op, line int) error {
	switch op.Op {
	case batch.OpMeta:
		return nil
	case batch.OpDomainAdd:
		var a batch.DomainAddArgs
		if err := json.Unmarshal(op.Args, &a); err != nil {
			return fmt.Errorf("line %d: domain.add args: %w", line, err)
		}
		if a.ID == "" || len(a.Layers) == 0 {
			return fmt.Errorf("line %d: domain.add requires id and layers", line)
		}
	case batch.OpNodeAdd:
		var a batch.NodeAddArgs
		if err := json.Unmarshal(op.Args, &a); err != nil {
			return fmt.Errorf("line %d: node.add args: %w", line, err)
		}
		if a.Domain == "" || a.Layer == "" || a.Name == "" {
			return fmt.Errorf("line %d: node.add requires domain, layer, name", line)
		}
	case batch.OpNodeUpdate:
		var a batch.NodeUpdateArgs
		if err := json.Unmarshal(op.Args, &a); err != nil {
			return fmt.Errorf("line %d: node.update args: %w", line, err)
		}
		if a.ID == "" {
			return fmt.Errorf("line %d: node.update requires id", line)
		}
	case batch.OpNodeDelete:
		var a batch.NodeDeleteArgs
		if err := json.Unmarshal(op.Args, &a); err != nil {
			return fmt.Errorf("line %d: node.delete args: %w", line, err)
		}
		if a.ID == "" {
			return fmt.Errorf("line %d: node.delete requires id", line)
		}
	case batch.OpEdgeAdd:
		var a batch.EdgeAddArgs
		if err := json.Unmarshal(op.Args, &a); err != nil {
			return fmt.Errorf("line %d: edge.add args: %w", line, err)
		}
		if a.Src == "" || a.Target == "" || a.Type == "" {
			return fmt.Errorf("line %d: edge.add requires src, target, type", line)
		}
	case batch.OpEdgeDelete:
		var a batch.EdgeDeleteArgs
		if err := json.Unmarshal(op.Args, &a); err != nil {
			return fmt.Errorf("line %d: edge.delete args: %w", line, err)
		}
		if a.ID == 0 {
			return fmt.Errorf("line %d: edge.delete requires id", line)
		}
	}
	return nil
}
