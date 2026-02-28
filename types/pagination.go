package types

import "github.com/nicolasparada/go-errs"

const maxPageSize = 200

type Page[T any] struct {
	Items    []T      `json:"items"`
	PageInfo PageInfo `json:"pageInfo"`
}

type PageInfo struct {
	EndCursor       *string `json:"endCursor"`
	HasNextPage     bool    `json:"hasNextPage"`
	StartCursor     *string `json:"startCursor"`
	HasPreviousPage bool    `json:"hasPreviousPage"`
}

type PageArgs struct {
	First  *uint
	After  *string
	Last   *uint
	Before *string
}

func (args PageArgs) IsBackwards() bool {
	return args.Last != nil || args.Before != nil
}

func (args *PageArgs) Validate() error {
	if args.First != nil && args.Last != nil {
		return errs.InvalidArgumentError("cannot specify both first and last")
	}

	if args.First != nil && *args.First < 1 {
		return errs.InvalidArgumentError("first must be greater than 0")
	}

	if args.Last != nil && *args.Last < 1 {
		return errs.InvalidArgumentError("last must be greater than 0")
	}

	if args.First != nil && *args.First > maxPageSize {
		return errs.InvalidArgumentError("first overflow")
	}

	if args.Last != nil && *args.Last > maxPageSize {
		return errs.InvalidArgumentError("last overflow")
	}

	return nil
}
