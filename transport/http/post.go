package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/matryer/way"
	"github.com/nicolasparada/nakama"
)

type createPostInput struct {
	Content   string
	SpoilerOf *string
	NSFW      bool
}

func (h *handler) createPost(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var in createPostInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		h.respondErr(w, errBadRequest)
		return
	}

	ti, err := h.svc.CreatePost(r.Context(), in.Content, in.SpoilerOf, in.NSFW)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	h.respond(w, ti, http.StatusCreated)
}

func (h *handler) posts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	last, _ := strconv.ParseUint(q.Get("last"), 10, 64)
	before := emptyStrPtr(q.Get("before"))
	pp, err := h.svc.Posts(ctx, way.Param(ctx, "username"), last, before)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	if pp == nil {
		pp = []nakama.Post{} // non null array
	}

	h.respond(w, paginatedRespBody{
		Items:     pp,
		EndCursor: pp.EndCursor(),
	}, http.StatusOK)
}

func (h *handler) post(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	postID := way.Param(ctx, "post_id")
	p, err := h.svc.Post(ctx, postID)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	h.respond(w, p, http.StatusOK)
}

func (h *handler) deletePost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	postID := way.Param(ctx, "post_id")
	err := h.svc.DeletePost(ctx, postID)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) togglePostLike(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	postID := way.Param(ctx, "post_id")
	out, err := h.svc.TogglePostLike(ctx, postID)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	h.respond(w, out, http.StatusOK)
}

type addPostReactionReqBody struct {
	Emoji string `json:"emoji"`
}

func (h *handler) addPostReaction(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var reqBody addPostReactionReqBody
	err := json.NewDecoder(r.Body).Decode(&reqBody)
	if err != nil {
		h.respondErr(w, errBadRequest)
		return
	}

	ctx := r.Context()
	postID := way.Param(ctx, "post_id")
	out, err := h.svc.AddPostReaction(ctx, postID, reqBody.Emoji)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	h.respond(w, out, http.StatusOK)
}

func (h *handler) togglePostSubscription(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	postID := way.Param(ctx, "post_id")
	out, err := h.svc.TogglePostSubscription(ctx, postID)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	h.respond(w, out, http.StatusOK)
}
