package batch

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

type ParseError struct {
	Line int
	Err  error
}

func (e *ParseError) Error() string { return fmt.Sprintf("line %d: %v", e.Line, e.Err) }
func (e *ParseError) Unwrap() error { return e.Err }

type Decoder struct {
	r    *bufio.Reader
	line int
}

func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{r: bufio.NewReader(r)}
}

func (d *Decoder) Next() (Op, error) {
	for {
		raw, err := d.r.ReadBytes('\n')
		if len(raw) == 0 && err == io.EOF {
			return Op{}, io.EOF
		}
		if err != nil && err != io.EOF {
			return Op{}, err
		}
		d.line++
		trimmed := trimSpace(raw)
		if len(trimmed) == 0 {
			if err == io.EOF {
				return Op{}, io.EOF
			}
			continue
		}
		var op Op
		if jerr := json.Unmarshal(trimmed, &op); jerr != nil {
			return Op{}, &ParseError{Line: d.line, Err: jerr}
		}
		if !IsKnownOp(op.Op) {
			return Op{}, &ParseError{Line: d.line, Err: fmt.Errorf("unknown op %q", op.Op)}
		}
		return op, nil
	}
}

func (d *Decoder) Line() int { return d.line }

type Encoder struct {
	w io.Writer
}

func NewEncoder(w io.Writer) *Encoder { return &Encoder{w: w} }

func (e *Encoder) emit(name OpName, args any) error {
	payload, err := json.Marshal(args)
	if err != nil {
		return fmt.Errorf("marshal args: %w", err)
	}
	line, err := json.Marshal(Op{Op: name, Args: payload})
	if err != nil {
		return fmt.Errorf("marshal op: %w", err)
	}
	line = append(line, '\n')
	_, err = e.w.Write(line)
	return err
}

func (e *Encoder) Meta(a MetaArgs) error             { return e.emit(OpMeta, a) }
func (e *Encoder) DomainAdd(a DomainAddArgs) error   { return e.emit(OpDomainAdd, a) }
func (e *Encoder) NodeAdd(a NodeAddArgs) error       { return e.emit(OpNodeAdd, a) }
func (e *Encoder) NodeUpdate(a NodeUpdateArgs) error { return e.emit(OpNodeUpdate, a) }
func (e *Encoder) NodeDelete(a NodeDeleteArgs) error { return e.emit(OpNodeDelete, a) }
func (e *Encoder) EdgeAdd(a EdgeAddArgs) error       { return e.emit(OpEdgeAdd, a) }
func (e *Encoder) EdgeDelete(a EdgeDeleteArgs) error { return e.emit(OpEdgeDelete, a) }

func trimSpace(b []byte) []byte {
	start := 0
	for start < len(b) && (b[start] == ' ' || b[start] == '\t' || b[start] == '\r' || b[start] == '\n') {
		start++
	}
	end := len(b)
	for end > start && (b[end-1] == ' ' || b[end-1] == '\t' || b[end-1] == '\r' || b[end-1] == '\n') {
		end--
	}
	return b[start:end]
}
