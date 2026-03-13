package cockroach

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgxutil"
	"github.com/nakamauwu/nakama/types"
)

const notificationsCols = `
	  notifications.id
	, notifications.user_id
	, notifications.actor_usernames
	, notifications.kind
	, notifications.post_id
	, notifications.read_at
	, notifications.issued_at
	, (notifications.read_at IS NOT NULL AND notifications.read_at != '0001-01-01 00:00:00') AS read
`

func (c *Cockroach) Notifications(ctx context.Context, in types.ListNotifications) (types.Page[types.Notification], error) {
	var out types.Page[types.Notification]

	args := pgx.StrictNamedArgs{"user_id": in.UserID()}
	filters := []string{"notifications.user_id = @user_id"}

	pageArgs, err := ParsePageArgs[time.Time](in.PageArgs)
	if err != nil {
		return out, err
	}

	if pageArgs.After != nil {
		filters = append(filters, "(notifications.issued_at, notifications.id) < (@after_issued_at, @after_id)")
		args["after_issued_at"] = pageArgs.After.Value
		args["after_id"] = pageArgs.After.ID
	} else if pageArgs.Before != nil {
		filters = append(filters, "(notifications.issued_at, notifications.id) > (@before_issued_at, @before_id)")
		args["before_issued_at"] = pageArgs.Before.Value
		args["before_id"] = pageArgs.Before.ID
	}

	var order, limit string
	if pageArgs.IsBackwards() {
		order = "ORDER BY notifications.issued_at ASC, notifications.id ASC"
		limit = fmt.Sprintf("LIMIT %d", or(pageArgs.Last, defaultPageSize)+1) // +1 to check if there's a next page
	} else {
		order = "ORDER BY notifications.issued_at DESC, notifications.id DESC"
		limit = fmt.Sprintf("LIMIT %d", or(pageArgs.First, defaultPageSize)+1) // +1 to check if there's a next page
	}

	query := fmt.Sprintf(`
		SELECT %s
		FROM notifications
		WHERE %s
		%s
		%s
	`, notificationsCols,
		strings.Join(filters, " AND "),
		order,
		limit,
	)

	notifications, err := pgxutil.Select(ctx, c.db, query, []any{args}, pgx.RowToStructByNameLax[types.Notification])
	if err != nil {
		return out, fmt.Errorf("sql select notifications: %w", err)
	}

	out.Items = notifications

	return out, applyPageInfo(&out, pageArgs, func(n types.Notification) Cursor[time.Time] {
		return Cursor[time.Time]{ID: n.ID, Value: n.IssuedAt}
	})
}
