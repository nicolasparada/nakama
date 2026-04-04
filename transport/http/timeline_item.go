package http

import (
	"mime"
	"net/http"

	"github.com/matryer/way"

	"github.com/nakamauwu/nakama/types"
)

func (h *handler) timeline(w http.ResponseWriter, r *http.Request) {
	if a, _, err := mime.ParseMediaType(r.Header.Get("Accept")); err == nil && a == "text/event-stream" {
		h.timelineItemStream(w, r)
		return
	}

	ctx := r.Context()
	q := r.URL.Query()
	pageArgs, err := parsePageArgs(q)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	in := types.ListTimeline{
		PageArgs: pageArgs,
	}
	page, err := h.svc.Timeline(ctx, in)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	if page.Items == nil {
		page.Items = []types.TimelineItem{} // non null array
	}

	for i := range page.Items {
		if page.Items[i].Post.Reactions == nil {
			page.Items[i].Post.Reactions = []types.Reaction{} // non null array
		}
		if page.Items[i].Post.Media == nil {
			page.Items[i].Post.Media = []types.Media{} // non null array
		}
	}

	h.respond(w, page, http.StatusOK)
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
			ti.Post.Reactions = []types.Reaction{} // non null array
		}
		if ti.Post.Media == nil {
			ti.Post.Media = []types.Media{} // non null array
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
