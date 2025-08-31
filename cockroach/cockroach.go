package cockroach

import (
	"embed"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nicolasparada/go-db"
	"github.com/nicolasparada/nakama/ptr"
	"github.com/nicolasparada/nakama/types"
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

func addWhere(query string, filter string) string {
	if strings.Contains(strings.ToLower(query), "where") {
		return query + " AND " + filter
	}

	return query + " WHERE " + filter
}

func addPageFilter(query, table string, queryArgs pgx.StrictNamedArgs, pageArgs types.PageArgs) string {
	if pageArgs.After != nil {
		query = addWhere(query, table+".id < @after_id")
		queryArgs["after_id"] = *pageArgs.After
	}

	if pageArgs.Before != nil {
		query = addWhere(query, table+".id > @before_id")
		queryArgs["before_id"] = *pageArgs.Before
	}

	return query
}

func addPageOrder(query, table string, pageArgs types.PageArgs) string {
	if pageArgs.IsBackwards() {
		query += " ORDER BY " + table + ".id ASC"
	} else {
		query += " ORDER BY " + table + ".id DESC"
	}
	return query
}

func addLimit(query string, queryArgs pgx.StrictNamedArgs, pageArgs types.PageArgs) string {
	if pageArgs.IsBackwards() {
		query += " LIMIT COALESCE(@last, 10) + 1"
		queryArgs["last"] = pageArgs.Last
	} else {
		query += " LIMIT COALESCE(@first, 10) + 1"
		queryArgs["first"] = pageArgs.First
	}
	return query
}

func addLimitAndOffset(query string, queryArgs pgx.StrictNamedArgs, pageArgs types.SimplePageArgs) string {
	if pageArgs.PerPage == nil {
		pageArgs.PerPage = ptr.From(uint32(10))
	}

	query += " LIMIT @per_page + 1"
	queryArgs["per_page"] = *pageArgs.PerPage

	if pageArgs.Page != nil && *pageArgs.Page > 1 {
		offset := (*pageArgs.Page - 1) * (*pageArgs.PerPage)
		query += " OFFSET @offset"
		queryArgs["offset"] = offset
	}

	return query
}

func applyPageInfo[T any](page *types.Page[T], pageArgs types.PageArgs, cursorFunc func(item T) string) {
	l := uint32(len(page.Items))
	if l == 0 {
		return
	}

	backwards := pageArgs.IsBackwards()
	if backwards {
		last := ptr.Or(pageArgs.Last, 10)
		page.PageInfo.HasPrevPage = l > last
		if page.PageInfo.HasPrevPage {
			page.Items = page.Items[:last]
		}
		page.PageInfo.HasNextPage = pageArgs.Before != nil
	} else {
		first := ptr.Or(pageArgs.First, 10)
		page.PageInfo.HasNextPage = l > first
		if page.PageInfo.HasNextPage {
			page.Items = page.Items[:first]
		}
		page.PageInfo.HasPrevPage = pageArgs.After != nil
	}

	if backwards {
		slices.Reverse(page.Items)
	}

	l = uint32(len(page.Items))
	if l == 0 {
		return
	}

	page.PageInfo.StartCursor = ptr.From(cursorFunc(page.Items[0]))
	page.PageInfo.EndCursor = ptr.From(cursorFunc(page.Items[l-1]))
}

func applySimplePageInfo[T any](page *types.SimplePage[T], pageArgs types.SimplePageArgs) {
	l := uint32(len(page.Items))
	if l == 0 {
		return
	}

	if pageArgs.PerPage == nil {
		pageArgs.PerPage = ptr.From(uint32(10))
	}

	page.PageInfo.CurrentPage = pageArgs.PageOrDefault()
	page.PageInfo.HasPrevPage = page.PageInfo.CurrentPage > 1
	page.PageInfo.HasNextPage = l > *pageArgs.PerPage

	if page.PageInfo.HasPrevPage {
		prev := page.PageInfo.CurrentPage - 1
		page.PageInfo.PrevPage = &prev
	}

	if page.PageInfo.HasNextPage {
		next := page.PageInfo.CurrentPage + 1
		page.PageInfo.NextPage = &next

		page.Items = page.Items[:*pageArgs.PerPage]
	}
}
