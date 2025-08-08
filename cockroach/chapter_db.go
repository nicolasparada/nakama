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

func (c *Cockroach) CreateChapter(ctx context.Context, in types.CreateChapter) (types.Created, error) {
	var out types.Created

	const q = `
		INSERT INTO chapters (id, publication_id, title, content, number)
		VALUES (@chapter_id, @publication_id, @title, @content, @number)
		RETURNING id, created_at
	`

	rows, err := c.db.Query(ctx, q, pgx.StrictNamedArgs{
		"chapter_id":     id.Generate(),
		"publication_id": in.PublicationID,
		"title":          in.Title,
		"content":        in.Content,
		"number":         in.Number,
	})
	if db.IsUniqueViolationError(err, "number") {
		return out, errs.NewAlreadyExistsError("Number", "Chapter with this number already exists")
	}

	if err != nil {
		return out, fmt.Errorf("sql insert chapter: %w", err)
	}

	out, err = pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[types.Created])
	if err != nil {
		return out, fmt.Errorf("sql collect inserted chapter: %w", err)
	}

	return out, nil
}

func (c *Cockroach) Chapters(ctx context.Context, publicationID string) (types.Page[types.Chapter], error) {
	var out types.Page[types.Chapter]

	const q = `
		SELECT *
		FROM chapters
		WHERE publication_id = @publication_id
		ORDER BY number ASC
	`

	rows, err := c.db.Query(ctx, q, pgx.NamedArgs{"publication_id": publicationID})
	if err != nil {
		return out, fmt.Errorf("sql select chapters: %w", err)
	}

	out.Items, err = pgx.CollectRows(rows, pgx.RowToStructByNameLax[types.Chapter])
	if err != nil {
		return out, fmt.Errorf("sql collect chapters: %w", err)
	}

	return out, nil
}

func (c *Cockroach) Chapter(ctx context.Context, chapterID string) (types.Chapter, error) {
	var out types.Chapter

	const q = `
		SELECT chapters.*, to_json(publications) AS publication
		FROM chapters
		INNER JOIN publications ON chapters.publication_id = publications.id
		WHERE chapters.id = @chapter_id
	`

	rows, err := c.db.Query(ctx, q, pgx.NamedArgs{"chapter_id": chapterID})
	if err != nil {
		return out, fmt.Errorf("sql select chapter: %w", err)
	}

	out, err = pgx.CollectExactlyOneRow(rows, pgx.RowToStructByNameLax[types.Chapter])
	if db.IsNotFoundError(err) {
		return out, errs.NewNotFoundError("chapter not found")
	}
	if err != nil {
		return out, fmt.Errorf("sql collect chapter: %w", err)
	}

	return out, nil
}

func (c *Cockroach) LatestChapterNumber(ctx context.Context, publicationID string) (uint32, error) {
	var out uint32

	const q = `
		SELECT COALESCE(MAX(number), 0)
		FROM chapters
		WHERE publication_id = @publication_id
	`

	row := c.db.QueryRow(ctx, q, pgx.NamedArgs{"publication_id": publicationID})
	if err := row.Scan(&out); err != nil {
		return 0, fmt.Errorf("sql select latest chapter number: %w", err)
	}

	return out, nil
}
