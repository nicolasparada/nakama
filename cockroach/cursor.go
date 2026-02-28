package cockroach

import (
	"fmt"
	"slices"

	"github.com/btcsuite/btcutil/base58"
	"github.com/nakamauwu/nakama/types"
	"github.com/nicolasparada/go-errs"
	"github.com/vmihailenco/msgpack/v5"
)

const defaultPageSize = 3

type Cursor[T any] struct {
	ID string `msgpack:"i"`
	// Value will most of the time be [CreatedAt time.Time] field.
	Value T `msgpack:"v,omitempty"`
}

func EncodeCursor[T any](cursor Cursor[T]) (string, error) {
	b, err := msgpack.Marshal(cursor)
	if err != nil {
		return "", fmt.Errorf("msgpack marshal cursor: %w", err)
	}

	return base58.Encode(b), nil
}

func DecodeCursor[T any](s string) (Cursor[T], error) {
	var c Cursor[T]

	b := base58.Decode(s)
	if err := msgpack.Unmarshal(b, &c); err != nil {
		return c, errs.InvalidArgumentError("invalid cursor")
	}

	return c, nil
}

type PageArgs[T any] struct {
	First  *uint
	After  *Cursor[T]
	Last   *uint
	Before *Cursor[T]
}

func (args PageArgs[T]) IsBackwards() bool {
	return args.Last != nil || args.Before != nil
}

func ParsePageArgs[T any](in types.PageArgs) (PageArgs[T], error) {
	var out PageArgs[T]

	if in.After != nil {
		after, err := DecodeCursor[T](*in.After)
		if err != nil {
			return out, fmt.Errorf("decode after cursor: %w", err)
		}

		out.After = &after
	}

	if in.Before != nil {
		before, err := DecodeCursor[T](*in.Before)
		if err != nil {
			return out, fmt.Errorf("decode before cursor: %w", err)
		}

		out.Before = &before
	}

	out.First = in.First
	out.Last = in.Last

	return out, nil
}

// applyPageInfo modifies the given page in-place.
// This is due to it needs to cut the items slice back by one
// and also reverse it in case of backwards pagination.
func applyPageInfo[I, C any](page *types.Page[I], pageArgs PageArgs[C], cursorFunc func(item I) Cursor[C]) error {
	l := uint(len(page.Items))
	if l == 0 {
		return nil
	}

	backwards := pageArgs.IsBackwards()
	if backwards {
		last := or(pageArgs.Last, defaultPageSize)
		page.PageInfo.HasPreviousPage = l > last
		if page.PageInfo.HasPreviousPage {
			page.Items = page.Items[:last]
		}
		page.PageInfo.HasNextPage = pageArgs.Before != nil
	} else {
		first := or(pageArgs.First, defaultPageSize)
		page.PageInfo.HasNextPage = l > first
		if page.PageInfo.HasNextPage {
			page.Items = page.Items[:first]
		}
		page.PageInfo.HasPreviousPage = pageArgs.After != nil
	}

	if backwards {
		slices.Reverse(page.Items)
	}

	l = uint(len(page.Items))
	if l == 0 {
		return nil
	}

	startCursor := cursorFunc(page.Items[0])
	endCursor := cursorFunc(page.Items[l-1])

	if c, err := EncodeCursor(startCursor); err != nil {
		return fmt.Errorf("encode start cursor: %w", err)
	} else {
		page.PageInfo.StartCursor = new(c)
	}

	if c, err := EncodeCursor(endCursor); err != nil {
		return fmt.Errorf("encode end cursor: %w", err)
	} else {
		page.PageInfo.EndCursor = new(c)
	}

	return nil
}

func or[T any](a *T, b T) T {
	if a != nil {
		return *a
	}

	return b
}
