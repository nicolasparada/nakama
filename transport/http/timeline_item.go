package http

import (
	"encoding/json"
	"mime"
	"net/http"
	"strconv"

	"github.com/matryer/way"
	"github.com/nicolasparada/nakama"
)

type createTimelineItemInput struct {
	Content   string  `json:"content"`
	SpoilerOf *string `json:"spoilerOf"`
	NSFW      bool    `json:"nsfw"`
}

func (h *handler) createTimelineItem(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var in createTimelineItemInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		h.respondErr(w, errBadRequest)
		return
	}

	ti, err := h.svc.CreateTimelineItem(r.Context(), in.Content, in.SpoilerOf, in.NSFW)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	if ti.Post.Reactions == nil {
		ti.Post.Reactions = []nakama.Reaction{} // non null array
	}

	h.respond(w, ti, http.StatusCreated)
}

func (h *handler) timeline(w http.ResponseWriter, r *http.Request) {
	if a, _, err := mime.ParseMediaType(r.Header.Get("Accept")); err == nil && a == "text/event-stream" {
		h.timelineItemStream(w, r)
		return
	}

	ctx := r.Context()
	q := r.URL.Query()
	last, _ := strconv.ParseUint(q.Get("last"), 10, 64)
	before := emptyStrPtr(q.Get("before"))
	tt, err := h.svc.Timeline(ctx, last, before)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	if tt == nil {
		tt = []nakama.TimelineItem{} // non null array
	}

	for i := range tt {
		if tt[i].Post.Reactions == nil {
			tt[i].Post.Reactions = []nakama.Reaction{} // non null array
		}
	}

	h.respond(w, paginatedRespBody{
		Items:     tt,
		EndCursor: tt.EndCursor(),
	}, http.StatusOK)
}

func (h *handler) timelineItemStream(w http.ResponseWriter, r *http.Request) {
	f, ok := w.(http.Flusher)
	if !ok {
		h.respondErr(w, errStreamingUnsupported)
		return
	}

	ctx := r.Context()
	tt, err := h.svc.TimelineItemStream(ctx)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	header := w.Header()
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	header.Set("Content-Type", "text/event-stream; charset=utf-8")

	select {
	case ti := <-tt:
		if ti.Post.Reactions == nil {
			ti.Post.Reactions = []nakama.Reaction{} // non null array
		}

		h.writeSSE(w, ti)
		f.Flush()
	case <-ctx.Done():
		return
	}
}

func (h *handler) deleteTimelineItem(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	timelineItemID := way.Param(ctx, "timeline_item_id")
	err := h.svc.DeleteTimelineItem(ctx, timelineItemID)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
