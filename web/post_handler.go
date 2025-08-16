package web

import (
	"fmt"
	"io"
	"net/http"

	"github.com/nicolasparada/nakama/auth"
	"github.com/nicolasparada/nakama/types"
	"golang.org/x/sync/errgroup"
)

const storeInMemoryUntil = 10 * 1024 * 1024 // 10 MB

func (h *Handler) showHome(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()

	var feed types.Page[types.Post]
	if _, loggedIn := auth.UserFromContext(ctx); loggedIn && q.Get("feed") != "global" {
		var err error
		feed, err = h.Service.Feed(ctx, types.ListFeed{})
		if err != nil {
			h.renderWithError(w, r, "home.tmpl", nil, fmt.Errorf("fetch user feed: %w", err))
			return
		}
	} else {
		var err error
		feed, err = h.Service.Posts(ctx, types.ListPosts{})
		if err != nil {
			h.renderWithError(w, r, "home.tmpl", nil, fmt.Errorf("fetch general feed: %w", err))
			return
		}
	}

	h.render(w, r, "home.tmpl", map[string]any{
		"Feed": feed,
	}, http.StatusOK)
}

func (h *Handler) createPost(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	if err := r.ParseMultipartForm(storeInMemoryUntil); err != nil {
		h.redirectBackWithError(w, r, fmt.Errorf("parse multipart form: %w", err))
		return
	}

	defer r.MultipartForm.RemoveAll()

	var attachments []io.ReadSeeker
	for _, fileHeader := range r.MultipartForm.File["attachments"] {
		f, err := fileHeader.Open()
		if err != nil {
			h.redirectBackWithError(w, r, fmt.Errorf("open attachment file: %w", err))
			return
		}

		defer f.Close()

		attachments = append(attachments, f)
	}

	ctx := r.Context()
	in := types.CreatePost{
		Content:     r.PostFormValue("content"),
		IsR18:       r.PostFormValue("is_r18") == "on",
		Attachments: attachments,
	}
	_, err := h.Service.CreatePost(ctx, in)
	if err != nil {
		h.redirectBackWithError(w, r, fmt.Errorf("create post: %w", err))
		return
	}

	http.Redirect(w, r, r.Referer(), http.StatusSeeOther)
}

func (h *Handler) showPost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	postID := r.PathValue("postID")

	var (
		post     types.Post
		comments types.Page[types.Comment]
	)

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		var err error

		post, err = h.Service.Post(ctx, types.RetrievePost{PostID: postID})
		if err != nil {
			return fmt.Errorf("fetch post: %w", err)
		}
		return nil
	})

	g.Go(func() error {
		var err error
		comments, err = h.Service.Comments(gctx, postID)
		if err != nil {
			return fmt.Errorf("fetch comments: %w", err)
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		h.renderWithError(w, r, "post.tmpl", nil, err)
		return
	}

	h.render(w, r, "post.tmpl", map[string]any{
		"Post":     post,
		"Comments": comments,
	}, http.StatusOK)
}

func (h *Handler) toggleReaction(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	if err := r.ParseForm(); err != nil {
		h.redirectBackWithError(w, r, fmt.Errorf("parse form: %w", err))
		return
	}

	ctx := r.Context()
	postID := r.PathValue("postID")
	emoji := r.FormValue("emoji")

	// Debug logging
	if emoji == "" {
		h.redirectBackWithError(w, r, fmt.Errorf("emoji parameter is empty"))
		return
	}

	in := types.ToggleReaction{
		PostID: postID,
		Emoji:  emoji,
	}
	err := h.Service.ToggleReaction(ctx, in)
	if err != nil {
		h.redirectBackWithError(w, r, fmt.Errorf("toggle reaction: %w", err))
		return
	}

	http.Redirect(w, r, r.Referer(), http.StatusSeeOther)
}
