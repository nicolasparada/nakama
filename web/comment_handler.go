package web

import (
	"fmt"
	"net/http"

	"github.com/nicolasparada/nakama/types"
)

func (h *Handler) createComment(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	if err := r.ParseForm(); err != nil {
		h.redirectBackWithError(w, r, fmt.Errorf("parse form: %w", err))
		return
	}

	ctx := r.Context()
	in := types.CreateComment{
		PostID:  r.PathValue("postID"),
		Content: r.PostFormValue("content"),
	}
	_, err := h.Service.CreateComment(ctx, in)
	if err != nil {
		// TODO: save old form values to repopulate the form.
		h.redirectBackWithError(w, r, fmt.Errorf("create comment: %w", err))
		return
	}

	http.Redirect(w, r, r.Referer(), http.StatusSeeOther)
}
