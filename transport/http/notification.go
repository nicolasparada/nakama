package http

import (
	"mime"
	"net/http"
	"strconv"

	"github.com/matryer/way"
	"github.com/nakamauwu/nakama/types"
)

func (h *handler) notifications(w http.ResponseWriter, r *http.Request) {
	if a, _, err := mime.ParseMediaType(r.Header.Get("Accept")); err == nil && a == "text/event-stream" {
		h.notificationStream(w, r)
		return
	}

	q := r.URL.Query()
	last, _ := strconv.ParseUint(q.Get("last"), 10, 64)
	before := emptyStrPtr(q.Get("before"))
	nn, err := h.svc.Notifications(r.Context(), last, before)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	if nn == nil {
		nn = []types.Notification{} // non null array
	}

	h.respond(w, paginatedRespBody{
		Items:     nn,
		EndCursor: nn.EndCursor(),
	}, http.StatusOK)
}

func (h *handler) notificationStream(w http.ResponseWriter, r *http.Request) {
	f, ok := w.(http.Flusher)
	if !ok {
		h.respondErr(w, errStreamingUnsupported)
		return
	}

	ctx := r.Context()
	nn, err := h.svc.NotificationStream(ctx)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	header := w.Header()
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	header.Set("Content-Type", "text/event-stream; charset=utf-8")

	select {
	case n := <-nn:
		h.writeSSE(w, n)
		f.Flush()
	case <-ctx.Done():
		return
	}
}

func (h *handler) hasUnreadNotifications(w http.ResponseWriter, r *http.Request) {
	unread, err := h.svc.HasUnreadNotifications(r.Context())
	if err != nil {
		h.respondErr(w, err)
		return
	}

	h.respond(w, unread, http.StatusOK)
}

func (h *handler) markNotificationAsRead(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	notificationID := way.Param(ctx, "notification_id")
	err := h.svc.MarkNotificationAsRead(ctx, notificationID)
	if err != nil {
		h.respondErr(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) markNotificationsAsRead(w http.ResponseWriter, r *http.Request) {
	err := h.svc.MarkNotificationsAsRead(r.Context())
	if err != nil {
		h.respondErr(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
