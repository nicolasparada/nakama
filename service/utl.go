package service

import (
	"bytes"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"text/template"
)

const (
	minPageSize     = 1
	defaultPageSize = 10
	maxPageSize     = 99
)

var queriesCache sync.Map

func buildQuery(text string, data map[string]any) (string, []any, error) {
	var t *template.Template
	v, ok := queriesCache.Load(text)
	if !ok {
		var err error
		t, err = template.New("query").Parse(text)
		if err != nil {
			return "", nil, fmt.Errorf("could not parse sql query template: %w", err)
		}

		queriesCache.Store(text, t)
	} else {
		t = v.(*template.Template)
	}

	var wr bytes.Buffer
	if err := t.Execute(&wr, data); err != nil {
		return "", nil, fmt.Errorf("could not apply sql query data: %w", err)
	}

	query := wr.String()
	args := []any{}
	for key, val := range data {
		if !strings.Contains(query, "@"+key) {
			continue
		}

		args = append(args, val)
		query = strings.ReplaceAll(query, "@"+key, fmt.Sprintf("$%d", len(args)))
	}
	return query, args, nil
}

func normalizePageSize(i uint64) uint64 {
	if i == 0 {
		return defaultPageSize
	}
	if i < minPageSize {
		return minPageSize
	}
	if i > maxPageSize {
		return maxPageSize
	}
	return i
}

func cloneURL(u *url.URL) *url.URL {
	if u == nil {
		return nil
	}
	u2 := new(url.URL)
	*u2 = *u
	if u.User != nil {
		u2.User = new(url.Userinfo)
		*u2.User = *u.User
	}
	return u2
}
