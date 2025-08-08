package migrator

import (
	"context"
	"fmt"
	"io/fs"
	"slices"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nicolasparada/go-db"
)

func Migrate(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS) error {
	db := db.New(pool)
	// if err := ensureMigrationsTable(ctx, db); err != nil {
	// 	return err
	// }

	matches, err := fs.Glob(fsys, "**/*.sql")
	if err != nil {
		return err
	}

	slices.Sort(matches)

	return db.RunTx(ctx, func(ctx context.Context) error {
		for _, match := range matches {
			// name := strings.TrimSuffix(filepath.Base(match), ".sql")

			// exists, err := migrationExists(ctx, db, name)
			// if err != nil {
			// 	return err
			// }

			// if exists {
			// 	continue
			// }

			b, err := fs.ReadFile(fsys, match)
			if err != nil {
				return err
			}

			_, err = db.Exec(ctx, string(b))
			if err != nil {
				return err
			}

			// if err := recordMigration(ctx, db, name); err != nil {
			// 	return err
			// }
		}

		return nil
	})
}

func ensureMigrationsTable(ctx context.Context, db *db.DB) error {
	_, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS migrations (
			name VARCHAR NOT NULL PRIMARY KEY,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	if err != nil {
		return fmt.Errorf("sql create migrations table: %w", err)
	}
	return nil
}

func migrationExists(ctx context.Context, db *db.DB, name string) (bool, error) {
	var exists bool
	err := db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM migrations WHERE name = $1
		)
	`, name).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("sql check migration exists: %w", err)
	}
	return exists, nil
}

func recordMigration(ctx context.Context, db *db.DB, name string) error {
	_, err := db.Exec(ctx, `
		INSERT INTO migrations (name) VALUES ($1)
	`, name)
	if err != nil {
		return fmt.Errorf("sql record migration: %w", err)
	}
	return nil
}
