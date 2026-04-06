package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"syscall"

	"github.com/nakamauwu/nakama/service"
	"github.com/nakamauwu/nakama/types"
)

func (h *handler) userProfiles(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	pageArgs, err := parsePageArgs(q)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	in := types.ListUserProfiles{
		PageArgs: pageArgs,
	}

	if q.Has("search") {
		in.SearchUsername = new(q.Get("search"))
	}

	users, err := h.svc.UserProfiles(r.Context(), in)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	if users.Items == nil {
		users.Items = []types.UserProfile{} // non null array
	}

	h.respond(w, users, http.StatusOK)
}

func (h *handler) usernames(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	pageArgs, err := parsePageArgs(q)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	in := types.ListUsernames{
		StartingWith: q.Get("starting_with"),
		PageArgs:     pageArgs,
	}

	out, err := h.svc.Usernames(r.Context(), in)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	if out.Items == nil {
		out.Items = []string{} // non null array
	}

	h.respond(w, out, http.StatusOK)
}

func (h *handler) userProfileByUsername(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	username := r.PathValue("username")
	user, err := h.svc.UserProfileByUsername(ctx, types.RetrieveUserProfile{Username: username})
	if err != nil {
		h.respondErr(w, err)
		return
	}

	h.respond(w, user, http.StatusOK)
}

func (h *handler) updateUser(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var in types.UpdateUser
	err := json.NewDecoder(r.Body).Decode(&in)
	if err != nil {
		h.respondErr(w, errBadRequest)
		return
	}

	ctx := r.Context()
	err = h.svc.UpdateUser(ctx, in)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) updateAvatar(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, service.MaxAvatarBytes))
	if err != nil {
		h.respondErr(w, errBadRequest)
		return
	}

	avatarURL, err := h.svc.UpdateAvatar(r.Context(), bytes.NewReader(b))
	if err != nil {
		h.respondErr(w, err)
		return
	}

	_, err = fmt.Fprint(w, avatarURL)
	if err != nil && !errors.Is(err, syscall.EPIPE) {
		_ = h.logger.Log("err", fmt.Errorf("could not write avatar URL: %w", err))
		return
	}
}

func (h *handler) updateCover(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, service.MaxAvatarBytes))
	if err != nil {
		h.respondErr(w, errBadRequest)
		return
	}

	coverURL, err := h.svc.UpdateCover(r.Context(), bytes.NewReader(b))
	if err != nil {
		h.respondErr(w, err)
		return
	}

	_, err = fmt.Fprint(w, coverURL)
	if err != nil && !errors.Is(err, syscall.EPIPE) {
		_ = h.logger.Log("err", fmt.Errorf("could not write cover URL: %w", err))
		return
	}
}

func (h *handler) toggleFollow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	username := r.PathValue("username")

	out, err := h.svc.ToggleFollow(ctx, username)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	h.respond(w, out, http.StatusOK)
}

func (h *handler) followers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	pageArgs, err := parsePageArgs(q)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	in := types.ListFollowers{
		Username: r.PathValue("username"),
		PageArgs: pageArgs,
	}
	out, err := h.svc.Followers(ctx, in)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	if out.Items == nil {
		out.Items = []types.UserProfile{} // non null array
	}

	h.respond(w, out, http.StatusOK)
}

func (h *handler) followees(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	pageArgs, err := parsePageArgs(q)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	in := types.ListFollowees{
		Username: r.PathValue("username"),
		PageArgs: pageArgs,
	}
	out, err := h.svc.Followees(ctx, in)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	if out.Items == nil {
		out.Items = []types.UserProfile{} // non null array
	}

	h.respond(w, out, http.StatusOK)
}

func (h *handler) requestEmailUpdate(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var in types.RequestEmailUpdate
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		h.respondErr(w, errBadRequest)
		return
	}

	if err := h.svc.RequestEmailUpdate(r.Context(), in); err != nil {
		h.respondErr(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) verifyEmailUpdate(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var in struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		h.respondErr(w, errBadRequest)
		return
	}

	user, err := h.svc.VerifyEmailUpdate(r.Context(), in.Code)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	h.respond(w, user, http.StatusOK)
}
