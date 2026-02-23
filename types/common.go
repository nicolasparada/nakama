package types

import (
	"regexp"
	"time"
)

var reUUIDv4 = regexp.MustCompile("^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$")

func ValidUUIDv4(id string) bool {
	return reUUIDv4.MatchString(id)
}

type Created struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt" db:"created_at"`
}
