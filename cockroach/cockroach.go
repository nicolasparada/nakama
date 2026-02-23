package cockroach

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/cockroachdb/cockroach-go/v2/crdb"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nicolasparada/go-db"
)

//go:embed schema.sql
var Schema string

type Cockroach struct {
	db *db.DB
}

func New(pool *pgxpool.Pool) *Cockroach {
	return &Cockroach{db: db.New(pool)}
}

func (c *Cockroach) Migrate(ctx context.Context) error {
	_, err := c.db.Exec(ctx, Schema)
	if err != nil {
		return fmt.Errorf("sql exec migration: %w", err)
	}
	return nil
}

// TODO: remove once all service methods are migrated to cockroach pkg.
func ExecuteTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}

	return crdb.ExecuteInTx(ctx, pgxTxAdapter{tx}, func() error {
		return fn(tx)
	})
}

type pgxTxAdapter struct {
	pgx.Tx
}

// Exec implements crdb.Tx interface.
func (a pgxTxAdapter) Exec(ctx context.Context, q string, args ...any) error {
	_, err := a.Tx.Exec(ctx, q, args...)
	return err
}
