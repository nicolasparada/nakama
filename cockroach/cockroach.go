package cockroach

import (
	"embed"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nicolasparada/go-db"
)

//go:embed migrations/*.sql
var MigrationsFS embed.FS

type Cockroach struct {
	db *db.DB
}

func New(pool *pgxpool.Pool) *Cockroach {
	return &Cockroach{
		db: db.New(pool),
	}
}
