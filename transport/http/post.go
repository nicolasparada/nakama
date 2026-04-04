package http

import (
	"encoding/json"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/matryer/way"

	"github.com/nakamauwu/nakama/service"
	"github.com/nakamauwu/nakama/types"
)

func (h *handler) createPost(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var in types.CreatePost

	var closeFuncs []func() error

	defer func() {
		for _, f := range closeFuncs {
			_ = f()
		}
	}()

	mediatype, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err == nil && strings.Contains(strings.ToLower(mediatype), "multipart/form-data") {
		if err := r.ParseMultipartForm(service.MaxMediaItemBytes); err != nil {
			h.respondErr(w, errBadRequest)
			return
		}

		defer r.MultipartForm.RemoveAll()

		in.Content = r.FormValue("content")
		if s := strings.TrimSpace(r.FormValue("spoiler_of")); s != "" {
			in.SpoilerOf = &s
		}
		if v, err := strconv.ParseBool(r.FormValue("nsfw")); err == nil {
			in.NSFW = v
		}
		if files, ok := r.MultipartForm.File["media"]; ok {
			for _, header := range files {
				if header.Size > service.MaxMediaItemBytes {
					h.respondErr(w, service.ErrMediaItemTooLarge)
					return
				}

				f, err := header.Open()
				if err != nil {
					h.respondErr(w, errBadRequest)
					return
				}

				closeFuncs = append(closeFuncs, f.Close)

				in.MediaReaders = append(in.MediaReaders, f)
			}
		}
	} else {
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			h.respondErr(w, errBadRequest)
			return
		}
	}

	ti, err := h.svc.CreatePost(r.Context(), in)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	if ti.Post.Reactions == nil {
		ti.Post.Reactions = []types.Reaction{} // non null array
	}

	if ti.Post.Media == nil {
		ti.Post.Media = []types.Media{} // non null array
	}

	h.respond(w, ti, http.StatusCreated)
}

func (h *handler) posts(w http.ResponseWriter, r *http.Request) {
	// SSE support only for /api/posts endpoint
	if r.URL.Path == "/api/posts" {
		if a, _, err := mime.ParseMediaType(r.Header.Get("Accept")); err == nil && a == "text/event-stream" {
			h.postStream(w, r)
			return
		}
	}

	ctx := r.Context()
	q := r.URL.Query()
	pageArgs, err := parsePageArgs(q)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	in := types.ListPosts{
		PageArgs: pageArgs,
	}

	// Username is an optional path parameter since this handler is used for both:
	// - /api/posts
	// - /api/users/:username/posts
	if username := way.Param(ctx, "username"); username != "" {
		in.Username = &username
	}

	if q.Has("tag") {
		in.Tag = new(q.Get("tag"))
	}

	page, err := h.svc.Posts(ctx, in)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	if page.Items == nil {
		page.Items = []types.Post{} // non null array
	}

	for i := range page.Items {
		if page.Items[i].Reactions == nil {
			page.Items[i].Reactions = []types.Reaction{} // non null array
		}
		if page.Items[i].Media == nil {
			page.Items[i].Media = []types.Media{} // non null array
		}
	}

	h.respond(w, page, http.StatusOK)
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
		if p.Media == nil {
			p.Media = []types.Media{} // non null array
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
	if p.Media == nil {
		p.Media = []types.Media{} // non null array
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
	in.ID = way.Param(ctx, "post_id")
	out, err := h.svc.UpdatePost(ctx, in)
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

	var in types.TogglePostReaction
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		h.respondErr(w, errBadRequest)
		return
	}

	ctx := r.Context()
	in.PostID = way.Param(ctx, "post_id")
	out, err := h.svc.TogglePostReaction(ctx, in)
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
