package types

import "time"

type Created struct {
	ID        string    `db:"id"`
	CreatedAt time.Time `db:"created_at"`
}

type Page[T any] struct {
	Items    []T
	PageInfo PageInfo
}

type PageInfo struct {
	StartCursor *string
	EndCursor   *string
	HasNextPage bool
	HasPrevPage bool
}

type PageArgs struct {
	First  *uint32
	Last   *uint32
	After  *string
	Before *string
}

func (in PageArgs) IsBackwards() bool {
	return in.Before != nil || in.Last != nil
}

type SimplePage[T any] struct {
	Items    []T
	PageInfo SimplePageInfo
}

type SimplePageInfo struct {
	HasNextPage bool
	HasPrevPage bool
	CurrentPage uint32
	PrevPage    *uint32
	NextPage    *uint32
}

type SimplePageArgs struct {
	Page    *uint32
	PerPage *uint32
}

func (args SimplePageArgs) PageOrDefault() uint32 {
	if args.Page == nil || *args.Page == 0 {
		return 1
	}
	return *args.Page
}
