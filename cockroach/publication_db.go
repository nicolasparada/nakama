package cockroach

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/nicolasparada/go-db"
	"github.com/nicolasparada/nakama/errs"
	"github.com/nicolasparada/nakama/id"
	"github.com/nicolasparada/nakama/types"
)

func (c *Cockroach) CreatePublication(ctx context.Context, in types.CreatePublication) (types.Created, error) {
	var out types.Created

	const q = `
		INSERT INTO publications (id, user_id, kind, title, description)
		VALUES (@publication_id, @user_id, @kind, @title, @description)
		RETURNING id, created_at
	`

	rows, err := c.db.Query(ctx, q, pgx.StrictNamedArgs{
		"publication_id": id.Generate(),
		"user_id":        in.UserID(),
		"kind":           in.Kind,
		"title":          in.Title,
		"description":    in.Description,
	})
	if err != nil {
		return out, fmt.Errorf("sql insert publication: %w", err)
	}

	out, err = pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[types.Created])
	if err != nil {
		return out, fmt.Errorf("sql collect inserted publication: %w", err)
	}

	return out, nil
}

func (c *Cockroach) Publications(ctx context.Context, in types.ListPublications) (types.Page[types.Publication], error) {
	var out types.Page[types.Publication]

	q := `
		SELECT publications.*, to_json(users) AS user
		FROM publications
		INNER JOIN users ON publications.user_id = users.id
		WHERE publications.kind = @kind
		ORDER BY id DESC
	`

	rows, err := c.db.Query(ctx, q, pgx.StrictNamedArgs{
		"kind": in.Kind,
	})
	if err != nil {
		return out, fmt.Errorf("sql select publications: %w", err)
	}

	out.Items, err = pgx.CollectRows(rows, pgx.RowToStructByNameLax[types.Publication])
	if err != nil {
		return out, fmt.Errorf("sql collect publications: %w", err)
	}

	return out, nil
}

func (c *Cockroach) Publication(ctx context.Context, publicationID string) (types.Publication, error) {
	var out types.Publication

	query := `
		SELECT publications.*, to_json(users) AS user
		FROM publications
		INNER JOIN users ON publications.user_id = users.id
		WHERE publications.id = @publication_id
	`

	rows, err := c.db.Query(ctx, query, pgx.StrictNamedArgs{
		"publication_id": publicationID,
	})
	if err != nil {
		return out, fmt.Errorf("sql select publication: %w", err)
	}

	out, err = pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[types.Publication])
	if db.IsNotFoundError(err) {
		return out, errs.NewNotFoundError("publication not found")
	}
	if err != nil {
		return out, fmt.Errorf("sql collect publication: %w", err)
	}

	return out, nil
}
