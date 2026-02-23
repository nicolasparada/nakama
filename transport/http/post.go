package http

import (
	"encoding/json"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/matryer/way"

	"github.com/nakamauwu/nakama/types"
)

func (h *handler) userPosts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	username := way.Param(ctx, "username")
	q := r.URL.Query()
	last, _ := strconv.ParseUint(q.Get("last"), 10, 64)
	before := emptyStrPtr(q.Get("before"))
	pp, err := h.svc.Posts(ctx, last, before, types.PostsFromUser(username))
	if err != nil {
		h.respondErr(w, err)
		return
	}

	if pp == nil {
		pp = []types.Post{} // non null array
	}

	for i := range pp {
		if pp[i].Reactions == nil {
			pp[i].Reactions = []types.Reaction{} // non null array
		}
		if pp[i].MediaURLs == nil {
			pp[i].MediaURLs = []string{} // non null array
		}
	}

	h.respond(w, paginatedRespBody{
		Items:     pp,
		EndCursor: pp.EndCursor(),
	}, http.StatusOK)
}

func (h *handler) posts(w http.ResponseWriter, r *http.Request) {
	if a, _, err := mime.ParseMediaType(r.Header.Get("Accept")); err == nil && a == "text/event-stream" {
		h.postStream(w, r)
		return
	}

	ctx := r.Context()
	q := r.URL.Query()
	last, _ := strconv.ParseUint(q.Get("last"), 10, 64)
	before := emptyStrPtr(q.Get("before"))

	var opts []types.PostsOpt
	if tag := strings.TrimSpace(q.Get("tag")); tag != "" {
		opts = append(opts, types.PostsTagged(tag))
	}
	pp, err := h.svc.Posts(ctx, last, before, opts...)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	if pp == nil {
		pp = []types.Post{} // non null array
	}

	for i := range pp {
		if pp[i].Reactions == nil {
			pp[i].Reactions = []types.Reaction{} // non null array
		}
		if pp[i].MediaURLs == nil {
			pp[i].MediaURLs = []string{} // non null array
		}
	}

	h.respond(w, paginatedRespBody{
		Items:     pp,
		EndCursor: pp.EndCursor(),
	}, http.StatusOK)
}

func (h *handler) postStream(w http.ResponseWriter, r *http.Request) {
	f, ok := w.(http.Flusher)
	if !ok {
		h.respondErr(w, errStreamingUnsupported)
		return
	}

	ctx := r.Context()
	pp, err := h.svc.PostStream(ctx)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	header := w.Header()
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	header.Set("Content-Type", "text/event-stream; charset=utf-8")

	select {
	case p := <-pp:
		if p.Reactions == nil {
			p.Reactions = []types.Reaction{} // non null array
		}
		if p.MediaURLs == nil {
			p.MediaURLs = []string{} // non null array
		}

		h.writeSSE(w, p)
		f.Flush()
	case <-ctx.Done():
		return
	}
}

func (h *handler) post(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	postID := way.Param(ctx, "post_id")
	p, err := h.svc.Post(ctx, postID)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	if p.Reactions == nil {
		p.Reactions = []types.Reaction{} // non null array
	}
	if p.MediaURLs == nil {
		p.MediaURLs = []string{} // non null array
	}

	h.respond(w, p, http.StatusOK)
}

func (h *handler) updatePost(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var in types.UpdatePost
	err := json.NewDecoder(r.Body).Decode(&in)
	if err != nil {
		h.respondErr(w, errBadRequest)
		return
	}

	ctx := r.Context()
	postID := way.Param(ctx, "post_id")
	out, err := h.svc.UpdatePost(ctx, postID, in)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	h.respond(w, out, http.StatusOK)
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

func (h *handler) togglePostReaction(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var in types.ReactionInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		h.respondErr(w, errBadRequest)
		return
	}

	ctx := r.Context()
	postID := way.Param(ctx, "post_id")
	out, err := h.svc.TogglePostReaction(ctx, postID, in)
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
