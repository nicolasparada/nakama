package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"syscall"

	"github.com/matryer/way"

	"github.com/nakamauwu/nakama/service"
	"github.com/nakamauwu/nakama/types"
)

func (h *handler) users(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	search := q.Get("search")
	first, _ := strconv.ParseUint(q.Get("first"), 10, 64)
	after := emptyStrPtr(q.Get("after"))
	uu, err := h.svc.Users(r.Context(), search, first, after)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	if uu == nil {
		uu = []types.UserProfile{} // non null array
	}

	h.respond(w, paginatedRespBody{
		Items:     uu,
		EndCursor: uu.EndCursor(),
	}, http.StatusOK)
}

func (h *handler) usernames(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	startingWith := q.Get("starting_with")
	first, _ := strconv.ParseUint(q.Get("first"), 10, 64)
	after := emptyStrPtr(q.Get("after"))
	uu, err := h.svc.Usernames(r.Context(), startingWith, first, after)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	if uu == nil {
		uu = []string{} // non null array
	}

	h.respond(w, paginatedRespBody{
		Items:     uu,
		EndCursor: uu.EndCursor(),
	}, http.StatusOK)
}

func (h *handler) user(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	username := way.Param(ctx, "username")
	u, err := h.svc.User(ctx, username)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	h.respond(w, u, http.StatusOK)
}

func (h *handler) updateUser(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var reqBody types.UpdateUserParams
	err := json.NewDecoder(r.Body).Decode(&reqBody)
	if err != nil {
		h.respondErr(w, errBadRequest)
		return
	}

	ctx := r.Context()
	err = h.svc.UpdateUser(ctx, reqBody)
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
	username := way.Param(ctx, "username")

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
	username := way.Param(ctx, "username")
	first, _ := strconv.ParseUint(q.Get("first"), 10, 64)
	after := emptyStrPtr(q.Get("after"))
	uu, err := h.svc.Followers(ctx, username, first, after)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	if uu == nil {
		uu = []types.UserProfile{} // non null array
	}

	h.respond(w, paginatedRespBody{
		Items:     uu,
		EndCursor: uu.EndCursor(),
	}, http.StatusOK)
}

func (h *handler) followees(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	username := way.Param(ctx, "username")
	first, _ := strconv.ParseUint(q.Get("first"), 10, 64)
	after := emptyStrPtr(q.Get("after"))
	uu, err := h.svc.Followees(ctx, username, first, after)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	if uu == nil {
		uu = []types.UserProfile{} // non null array
	}

	h.respond(w, paginatedRespBody{
		Items:     uu,
		EndCursor: uu.EndCursor(),
	}, http.StatusOK)
}
