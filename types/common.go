package types

import "time"

type Created struct {
	ID        string    `db:"id"`
	CreatedAt time.Time `db:"created_at"`
}

type Page[T any] struct {
	Items []T
}
