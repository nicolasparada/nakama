package web

import (
	"fmt"
	"net/http"

	"github.com/nicolasparada/nakama/types"
	"golang.org/x/sync/errgroup"
)

func (h *Handler) showUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	username := r.PathValue("username")
	q := r.URL.Query()

	pageArgs, err := parsePageArgs(q)
	if err != nil {
		h.renderErrorPage(w, r, fmt.Errorf("parse page args: %w", err))
		return
	}

	var (
		user  types.User
		posts types.Page[types.Post]
	)

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		var err error
		user, err = h.Service.UserFromUsername(gctx, types.RetrieveUserFromUsername{
			Username: username,
		})
		if err != nil {
			return fmt.Errorf("fetch user from username: %w", err)
		}
		return nil
	})

	g.Go(func() error {
		var err error
		posts, err = h.Service.Posts(gctx, types.ListPosts{
			Username: &username,
			PageArgs: pageArgs,
		})
		if err != nil {
			return fmt.Errorf("fetch user posts: %w", err)
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		h.renderErrorPage(w, r, fmt.Errorf("fetch user and posts: %w", err))
		return
	}

	h.render(w, r, "user.tmpl", map[string]any{
		"User":  user,
		"Posts": posts,
	}, http.StatusOK)
}

func (h *Handler) toggleFollow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := r.PathValue("userID")

	err := h.Service.ToggleFollow(ctx, types.ToggleFollow{
		FolloweeID: userID,
	})
	if err != nil {
		h.renderWithError(w, r, "user.tmpl", nil, fmt.Errorf("toggle follow: %w", err))
		return
	}

	http.Redirect(w, r, r.Referer(), http.StatusSeeOther)
}

func (h *Handler) showEditUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	username := r.PathValue("username")

	user, err := h.Service.UserFromUsername(ctx, types.RetrieveUserFromUsername{
		Username: username,
	})
	if err != nil {
		h.renderErrorPage(w, r, fmt.Errorf("fetch user for edit: %w", err))
		return
	}

	h.render(w, r, "edit_user.tmpl", map[string]any{
		"User": user,
	}, http.StatusOK)
}

func (h *Handler) uploadAvatar(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	if err := r.ParseMultipartForm(storeInMemoryUntil); err != nil {
		h.redirectBackWithError(w, r, fmt.Errorf("parse multipart form: %w", err))
		return
	}

	defer r.MultipartForm.RemoveAll()

	avatar, _, err := r.FormFile("avatar")
	if err != nil {
		h.redirectBackWithError(w, r, fmt.Errorf("read avatar file: %w", err))
		return
	}
	defer avatar.Close()

	ctx := r.Context()
	err = h.Service.UploadAvatar(ctx, avatar)
	if err != nil {
		h.redirectBackWithError(w, r, fmt.Errorf("upload avatar: %w", err))
		return
	}

	http.Redirect(w, r, r.Referer(), http.StatusSeeOther)
}
