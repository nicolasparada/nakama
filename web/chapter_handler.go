package web

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/nicolasparada/nakama/types"
	"golang.org/x/sync/errgroup"
)

func (h *Handler) showCreateChapter(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	publicationID := r.PathValue("publicationID")

	var (
		publication         types.Publication
		latestChapterNumber uint32
	)

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		var err error
		publication, err = h.Service.Publication(gctx, publicationID)
		if err != nil {
			return fmt.Errorf("fetch publication: %w", err)
		}
		return nil
	})

	g.Go(func() error {
		var err error
		latestChapterNumber, err = h.Service.LatestChapterNumber(gctx, publicationID)
		if err != nil {
			return fmt.Errorf("fetch latest chapter number: %w", err)
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		h.renderWithError(w, r, "new_chapter.tmpl", nil, err)
		return
	}

	h.render(w, r, "new_chapter.tmpl", map[string]any{
		"Publication":     publication,
		"SuggestedNumber": latestChapterNumber + 1,
	}, http.StatusOK)
}

func (h *Handler) createChapter(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	if err := r.ParseForm(); err != nil {
		h.redirectBackWithError(w, r, fmt.Errorf("parse form: %w", err))
		return
	}

	number, err := strconv.ParseUint(r.PostFormValue("number"), 10, 32)
	if err != nil {
		h.redirectBackWithError(w, r, fmt.Errorf("parse chapter number: %w", err))
		return
	}

	ctx := r.Context()
	publicationID := r.PathValue("publicationID")
	in := types.CreateChapter{
		PublicationID: publicationID,
		Title:         r.PostFormValue("title"),
		Content:       r.PostFormValue("content"),
		Number:        uint32(number),
	}
	out, err := h.Service.CreateChapter(ctx, in)
	if err != nil {
		h.redirectBackWithError(w, r, fmt.Errorf("create chapter: %w", err))
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/publications/%s/chapters/%s", publicationID, out.ID), http.StatusSeeOther)
}
