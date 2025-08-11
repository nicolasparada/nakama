package web

import (
	"fmt"
	"net/http"

	"github.com/nicolasparada/nakama/types"
)

func (h *Handler) notifications(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	in := types.ListNotifications{}
	notifications, err := h.Service.Notifications(ctx, in)
	if err != nil {
		h.renderErrorPage(w, r, fmt.Errorf("fetch notifications: %w", err))
		return
	}

	h.render(w, r, "notifications.tmpl", map[string]any{
		"Notifications": notifications,
	}, http.StatusOK)
}

func (h *Handler) readNotification(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	ctx := r.Context()
	in := types.ReadNotification{
		NotificationID: r.PathValue("notificationID"),
	}
	err := h.Service.ReadNotification(ctx, in)
	if err != nil {
		h.redirectBackWithError(w, r, fmt.Errorf("read notification: %w", err))
		return
	}

	http.Redirect(w, r, r.Referer(), http.StatusSeeOther)
}
