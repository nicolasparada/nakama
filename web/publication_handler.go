package web

import (
	"fmt"
	"net/http"

	"github.com/nicolasparada/nakama/types"
	"golang.org/x/sync/errgroup"
)

func (h *Handler) showPublications(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	kind := types.PublicationKind(q.Get("kind"))

	publications, err := h.Service.Publications(ctx, types.ListPublications{
		Kind: kind,
	})
	if err != nil {
		h.renderWithError(w, r, "publications.tmpl", nil, fmt.Errorf("fetch publications: %w", err))
		return
	}

	h.render(w, r, "publications.tmpl", map[string]any{
		"Kind":         kind,
		"Publications": publications,
	}, http.StatusOK)
}

func (h *Handler) showCreatePublication(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	kind := types.PublicationKind(q.Get("kind"))
	h.render(w, r, "new_publication.tmpl", map[string]any{
		"Kind": kind,
	}, http.StatusOK)
}

func (h *Handler) createPublication(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	if err := r.ParseForm(); err != nil {
		h.redirectBackWithError(w, r, fmt.Errorf("parse form: %w", err))
		return
	}

	ctx := r.Context()
	in := types.CreatePublication{
		Kind:        types.PublicationKind(r.PostFormValue("kind")),
		Title:       r.PostFormValue("title"),
		Description: r.PostFormValue("description"),
	}
	out, err := h.Service.CreatePublication(ctx, in)
	if err != nil {
		h.redirectBackWithError(w, r, fmt.Errorf("create publication: %w", err))
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/publications/%s", out.ID), http.StatusSeeOther)
}

func (h *Handler) showPublication(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	publicationID := r.PathValue("publicationID")

	var (
		publication types.Publication
		chapters    types.Page[types.Chapter]
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
		chapters, err = h.Service.Chapters(gctx, publicationID)
		if err != nil {
			return fmt.Errorf("fetch chapters: %w", err)
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		h.renderWithError(w, r, "publication.tmpl", nil, err)
		return
	}

	h.render(w, r, "publication.tmpl", map[string]any{
		"Publication": publication,
		"Chapters":    chapters,
	}, http.StatusOK)
}
