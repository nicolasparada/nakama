package id

import "github.com/rs/xid"

func Generate() string {
	return xid.New().String()
}

func Valid(s string) bool {
	id, err := xid.FromString(s)
	if err != nil {
		return false
	}
	return !id.IsNil() && !id.IsZero()
}
