package web

import (
	"fmt"
	"net/http"

	"github.com/nicolasparada/nakama/types"
)

func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	query := q.Get("q")
	tab := q.Get("tab")

	if query == "" {
		h.render(w, r, "search.tmpl", map[string]any{
			"Query": query,
			"Tab":   tab,
		}, http.StatusOK)
		return
	}

	var results any
	switch tab {
	case "users", "":
		var err error
		results, err = h.Service.SearchUsers(ctx, types.SearchUsers{
			Query: query,
		})
		if err != nil {
			h.renderErrorPage(w, r, fmt.Errorf("search users: %w", err))
			return
		}

	case "posts":
		var err error
		results, err = h.Service.SearchPosts(ctx, types.SearchPosts{
			Query: query,
		})
		if err != nil {
			h.renderErrorPage(w, r, fmt.Errorf("search posts: %w", err))
			return
		}
	}

	h.render(w, r, "search.tmpl", map[string]any{
		"Query":   query,
		"Tab":     tab,
		"Results": results,
	}, http.StatusOK)
}
