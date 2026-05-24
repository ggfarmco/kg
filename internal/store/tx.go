package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/ggfarmco/kg/internal/graph"
)

type txKey struct{}

func (s *Store) InTx(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	if _, ok := ctx.Value(txKey{}).(*sql.Tx); ok {
		return graph.ErrNestedTransaction
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	ctxWithTx := context.WithValue(ctx, txKey{}, tx)

	defer func() {
		if r := recover(); r != nil {
			_ = tx.Rollback()
			panic(r)
		}
		if err != nil {
			_ = tx.Rollback()
			return
		}
		if cerr := tx.Commit(); cerr != nil {
			err = fmt.Errorf("commit: %w", cerr)
		}
	}()

	return fn(ctxWithTx)
}

func (s *Store) conn(ctx context.Context) DBTX {
	if tx, ok := ctx.Value(txKey{}).(*sql.Tx); ok {
		return tx
	}
	return s.db
}

func (s *Store) inTxOrConn(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := ctx.Value(txKey{}).(*sql.Tx); ok {
		return fn(ctx)
	}
	return s.InTx(ctx, fn)
}

func (s *Store) InTxOrConn(ctx context.Context, fn func(ctx context.Context) error) error {
	return s.inTxOrConn(ctx, fn)
}
