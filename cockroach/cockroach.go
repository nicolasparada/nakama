package cockroach

import (
	"embed"
	"strings"

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

func where(filters []string) string {
	if len(filters) == 0 {
		return ""
	}

	return " WHERE " + strings.Join(filters, " AND ") + " "
}
