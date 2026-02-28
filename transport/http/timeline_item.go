package http

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/matryer/way"

	"github.com/nakamauwu/nakama/service"
	"github.com/nakamauwu/nakama/types"
)

type createTimelineItemInput struct {
	Content   string          `json:"content"`
	SpoilerOf *string         `json:"spoilerOf"`
	NSFW      bool            `json:"nsfw"`
	Media     []io.ReadSeeker `json:"-"`
}

func (h *handler) createTimelineItem(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var in createTimelineItemInput

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

				in.Media = append(in.Media, f)
			}
		}
	} else {
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			h.respondErr(w, errBadRequest)
			return
		}
	}

	ti, err := h.svc.CreateTimelineItem(r.Context(), in.Content, in.SpoilerOf, in.NSFW, in.Media)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	if ti.Post.Reactions == nil {
		ti.Post.Reactions = []types.Reaction{} // non null array
	}

	if ti.Post.MediaURLs == nil {
		ti.Post.MediaURLs = []string{} // non null array
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
		tt = []types.TimelineItem{} // non null array
	}

	for i := range tt {
		if tt[i].Post.Reactions == nil {
			tt[i].Post.Reactions = []types.Reaction{} // non null array
		}
		if tt[i].Post.MediaURLs == nil {
			tt[i].Post.MediaURLs = []string{} // non null array
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
			ti.Post.Reactions = []types.Reaction{} // non null array
		}
		if ti.Post.MediaURLs == nil {
			ti.Post.MediaURLs = []string{} // non null array
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
