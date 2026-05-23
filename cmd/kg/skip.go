package main

import (
	"errors"
	"io"

	"github.com/ggfarmco/kg/internal/graph"
)

func handleMaybeSkip(w io.Writer, err error, ifNotExists bool) error {
	if err == nil {
		return nil
	}
	if ifNotExists && isAlreadyExists(err) {
		return writeOK(w, map[string]any{"skipped": true, "reason": "already_exists"})
	}
	return err
}

func isAlreadyExists(err error) bool {
	return errors.Is(err, graph.ErrDomainAlreadyExists) ||
		errors.Is(err, graph.ErrNodeAlreadyExists) ||
		errors.Is(err, graph.ErrEdgeAlreadyExists)
}
