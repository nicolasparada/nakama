package web

import (
	"fmt"
	"net/http"

	"github.com/nicolasparada/nakama/types"
)

func (h *Handler) showUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	username := r.PathValue("username")

	user, err := h.Service.UserFromUsername(ctx, types.RetrieveUserFromUsername{
		Username: username,
	})
	if err != nil {
		h.renderWithError(w, r, "user.tmpl", nil, fmt.Errorf("fetch user from username: %w", err))
		return
	}

	h.render(w, r, "user.tmpl", map[string]any{
		"User": user,
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
