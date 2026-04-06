package cockroach

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgxutil"
	"github.com/nakamauwu/nakama/types"
	"github.com/nicolasparada/go-db"
	"github.com/nicolasparada/go-errs"
)

const (
	sqlUserCols = `
		  users.id
		, users.username
		, users.avatar
	`
	// The JSON version uses `avatarURL` instead of `avatar`.
	sqlUserJSONB = `
		jsonb_build_object(
			'id', users.id,
			'username', users.username,
			'avatarURL', users.avatar
		) AS user
	`
	sqlUserProfileCols = `
		  users.id
		, users.email
		, users.username
		, users.avatar
		, users.cover
		, users.bio
		, users.waifu
		, users.husbando
		, users.followers_count
		, users.followees_count
	`
	sqlViewerFollowsUserJoin = `LEFT JOIN follows AS viewer_follows_user ON viewer_follows_user.follower_id = @viewer_id AND viewer_follows_user.followee_id = users.id`
	sqlUserFollowsViewerJoin = `LEFT JOIN follows AS user_follows_viewer ON user_follows_viewer.follower_id = users.id AND user_follows_viewer.followee_id = @viewer_id`
)

func appendViewerRelationshipFields(selects, joins []string) ([]string, []string) {
	selects = append(selects,
		`users.id = @viewer_id AS is_me`,
		`viewer_follows_user.follower_id IS NOT NULL AS followed_by_viewer`,
		`user_follows_viewer.follower_id IS NOT NULL AS follows_viewer`)
	joins = append(joins, sqlViewerFollowsUserJoin, sqlUserFollowsViewerJoin)

	return selects, joins
}

func (c *Cockroach) CreateUser(ctx context.Context, in types.CreateUser) (types.Created, error) {
	cols := []string{"email", "username"}
	vals := []string{"@email", "@username"}
	args := pgx.StrictNamedArgs{
		"email":    strings.ToLower(in.Email),
		"username": in.Username,
	}
	if in.Provider != nil {
		cols = append(cols, fmt.Sprintf("%s_provider_id", in.Provider.Name))
		vals = append(vals, "@provider_id")
		args["provider_id"] = in.Provider.ID
	}
	query := fmt.Sprintf(`
		INSERT INTO users (%s)
		VALUES (%s)
		RETURNING id, created_at
	`, strings.Join(cols, ", "), strings.Join(vals, ", "))
	out, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowToStructByNameLax[types.Created])
	if db.IsUniqueViolationError(err, "email") {
		return out, errs.ConflictError("email taken")
	}

	if db.IsUniqueViolationError(err, "username") {
		return out, errs.ConflictError("username taken")
	}

	if err != nil {
		return out, fmt.Errorf("sql insert user: %w", err)
	}

	return out, nil
}

func (c *Cockroach) UserProfiles(ctx context.Context, in types.ListUserProfiles) (types.Page[types.UserProfile], error) {
	var out types.Page[types.UserProfile]

	args := pgx.StrictNamedArgs{}
	selects := []string{sqlUserProfileCols}
	joins := []string{}
	filters := []string{}

	if in.SearchUsername != nil {
		args["search_username"] = *in.SearchUsername
		filters = append(filters, "users.username ILIKE '%' || @search_username || '%'")
	}

	if in.ViewerID() != nil {
		args["viewer_id"] = *in.ViewerID()
		selects, joins = appendViewerRelationshipFields(selects, joins)
	}

	pageArgs, err := ParsePageArgs[any](in.PageArgs)
	if err != nil {
		return out, err
	}

	if pageArgs.After != nil {
		filters = append(filters, "users.username < @after_username")
		args["after_username"] = pageArgs.After.ID // Cursor ID is the username in this case
	} else if pageArgs.Before != nil {
		filters = append(filters, "users.username > @before_username")
		args["before_username"] = pageArgs.Before.ID // Cursor ID is the username in this case
	}

	var order, limit string
	if pageArgs.IsBackwards() {
		order = "ORDER BY users.username ASC, users.id ASC"
		limit = fmt.Sprintf("LIMIT %d", or(pageArgs.Last, defaultPageSize)+1) // +1 to check if there's a next page
	} else {
		order = "ORDER BY users.username DESC, users.id DESC"
		limit = fmt.Sprintf("LIMIT %d", or(pageArgs.First, defaultPageSize)+1) // +1 to check if there's a next page
	}

	var condWhere string
	if len(filters) > 0 {
		condWhere = " WHERE " + strings.Join(filters, " AND ")
	}

	query := fmt.Sprintf(`
		SELECT %s
		FROM users
		%s
		%s
		%s
		%s`,
		strings.Join(selects, ",\n\t\t"),
		strings.Join(joins, "\n\t\t"),
		condWhere,
		order,
		limit,
	)

	users, err := pgxutil.Select(ctx, c.db, query, []any{args}, pgx.RowToStructByNameLax[types.UserProfile])
	if err != nil {
		return out, fmt.Errorf("sql select user profiles: %w", err)
	}

	out.Items = users

	return out, applyPageInfo(&out, pageArgs, userProfileCursor)
}

func (c *Cockroach) Followers(ctx context.Context, in types.ListFollowers) (types.Page[types.UserProfile], error) {
	var out types.Page[types.UserProfile]

	args := pgx.StrictNamedArgs{"username": in.Username}
	selects := []string{sqlUserProfileCols}
	joins := []string{
		`INNER JOIN users ON users.id = follows.follower_id`,
	}
	filters := []string{
		`follows.followee_id = (SELECT id FROM users WHERE username = @username)`,
	}

	if in.ViewerID() != nil {
		args["viewer_id"] = *in.ViewerID()
		selects, joins = appendViewerRelationshipFields(selects, joins)
	}

	pageArgs, err := ParsePageArgs[any](in.PageArgs)
	if err != nil {
		return out, err
	}

	if pageArgs.After != nil {
		filters = append(filters, "users.username < @after_username")
		args["after_username"] = pageArgs.After.ID // Cursor ID is the username in this case
	} else if pageArgs.Before != nil {
		filters = append(filters, "users.username > @before_username")
		args["before_username"] = pageArgs.Before.ID // Cursor ID is the username in this case
	}

	var order, limit string
	if pageArgs.IsBackwards() {
		order = "ORDER BY users.username ASC, users.id ASC"
		limit = fmt.Sprintf("LIMIT %d", or(pageArgs.Last, defaultPageSize)+1) // +1 to check if there's a next page
	} else {
		order = "ORDER BY users.username DESC, users.id DESC"
		limit = fmt.Sprintf("LIMIT %d", or(pageArgs.First, defaultPageSize)+1) // +1 to check if there's a next page
	}

	query := fmt.Sprintf(`
		SELECT %s
		FROM follows
		%s
		WHERE %s
		%s
		%s`,
		strings.Join(selects, ",\n\t\t"),
		strings.Join(joins, "\n\t\t"),
		strings.Join(filters, " AND "),
		order,
		limit,
	)

	users, err := pgxutil.Select(ctx, c.db, query, []any{args}, pgx.RowToStructByNameLax[types.UserProfile])
	if err != nil {
		return out, fmt.Errorf("sql select followers: %w", err)
	}

	out.Items = users

	return out, applyPageInfo(&out, pageArgs, userProfileCursor)
}

func (c *Cockroach) Followees(ctx context.Context, in types.ListFollowees) (types.Page[types.UserProfile], error) {
	var out types.Page[types.UserProfile]

	args := pgx.StrictNamedArgs{"username": in.Username}
	selects := []string{sqlUserProfileCols}
	joins := []string{
		`INNER JOIN users ON users.id = follows.followee_id`,
	}
	filters := []string{
		`follows.follower_id = (SELECT id FROM users WHERE username = @username)`,
	}

	if in.ViewerID() != nil {
		args["viewer_id"] = *in.ViewerID()
		selects, joins = appendViewerRelationshipFields(selects, joins)
	}

	pageArgs, err := ParsePageArgs[any](in.PageArgs)
	if err != nil {
		return out, err
	}

	if pageArgs.After != nil {
		filters = append(filters, "users.username < @after_username")
		args["after_username"] = pageArgs.After.ID // Cursor ID is the username in this case
	} else if pageArgs.Before != nil {
		filters = append(filters, "users.username > @before_username")
		args["before_username"] = pageArgs.Before.ID // Cursor ID is the username in this case
	}

	var order, limit string
	if pageArgs.IsBackwards() {
		order = "ORDER BY users.username ASC, users.id ASC"
		limit = fmt.Sprintf("LIMIT %d", or(pageArgs.Last, defaultPageSize)+1) // +1 to check if there's a next page
	} else {
		order = "ORDER BY users.username DESC, users.id DESC"
		limit = fmt.Sprintf("LIMIT %d", or(pageArgs.First, defaultPageSize)+1) // +1 to check if there's a next page
	}

	query := fmt.Sprintf(`
		SELECT %s
		FROM follows
		%s
		WHERE %s
		%s
		%s`,
		strings.Join(selects, ",\n\t\t"),
		strings.Join(joins, "\n\t\t"),
		strings.Join(filters, " AND "),
		order,
		limit,
	)

	users, err := pgxutil.Select(ctx, c.db, query, []any{args}, pgx.RowToStructByNameLax[types.UserProfile])
	if err != nil {
		return out, fmt.Errorf("sql select followees: %w", err)
	}

	out.Items = users

	return out, applyPageInfo(&out, pageArgs, userProfileCursor)
}

func (c *Cockroach) Usernames(ctx context.Context, in types.ListUsernames) (types.Page[string], error) {
	var out types.Page[string]

	args := pgx.StrictNamedArgs{"starting_with": in.StartingWith}
	filters := []string{"users.username ILIKE @starting_with || '%'"}

	if in.ViewerID() != nil {
		args["viewer_id"] = *in.ViewerID()
		filters = append(filters, `users.id != @viewer_id`)
	}

	pageArgs, err := ParsePageArgs[any](in.PageArgs)
	if err != nil {
		return out, err
	}

	if pageArgs.After != nil {
		filters = append(filters, "users.username < @after_username")
		args["after_username"] = pageArgs.After.ID // Cursor ID is the username in this case
	} else if pageArgs.Before != nil {
		filters = append(filters, "users.username > @before_username")
		args["before_username"] = pageArgs.Before.ID // Cursor ID is the username in this case
	}

	var order, limit string
	if pageArgs.IsBackwards() {
		order = "ORDER BY users.username ASC"
		limit = fmt.Sprintf("LIMIT %d", or(pageArgs.Last, defaultPageSize)+1) // +1 to check if there's a next page
	} else {
		order = "ORDER BY users.username DESC"
		limit = fmt.Sprintf("LIMIT %d", or(pageArgs.First, defaultPageSize)+1) // +1 to check if there's a next page
	}

	query := fmt.Sprintf(`
		SELECT users.username
		FROM users
		WHERE %s
		%s
		%s`,
		strings.Join(filters, " AND "),
		order,
		limit,
	)

	usernames, err := pgxutil.Select(ctx, c.db, query, []any{args}, pgx.RowTo[string])
	if err != nil {
		return out, fmt.Errorf("sql select usernames: %w", err)
	}

	out.Items = usernames

	return out, applyPageInfo(&out, pageArgs, func(username string) Cursor[any] {
		return Cursor[any]{ID: username}
	})
}

func (c *Cockroach) User(ctx context.Context, userID string) (types.User, error) {
	query := `SELECT ` + sqlUserCols + ` FROM users WHERE id = @user_id`
	args := pgx.StrictNamedArgs{"user_id": userID}
	user, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowToStructByNameLax[types.User])
	if errors.Is(err, pgx.ErrNoRows) {
		return user, errs.NotFoundError("user not found")
	}

	if err != nil {
		return user, fmt.Errorf("sql select user: %w", err)
	}

	return user, nil
}

func (c *Cockroach) UserByEmail(ctx context.Context, email string) (types.User, error) {
	query := `SELECT ` + sqlUserCols + ` FROM users WHERE email = @email`
	args := pgx.StrictNamedArgs{"email": email}
	user, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowToStructByNameLax[types.User])
	if errors.Is(err, pgx.ErrNoRows) {
		return user, errs.NotFoundError("user not found")
	}

	if err != nil {
		return user, fmt.Errorf("sql select user by email: %w", err)
	}

	return user, nil
}

func (c *Cockroach) UserByProviderOrEmail(ctx context.Context, provider types.Provider, email string) (types.User, error) {
	var user types.User
	return user, c.db.RunTx(ctx, func(ctx context.Context) error {
		var err error
		user, err = c.userByProvider(ctx, provider)
		if errors.Is(err, errs.NotFound) {
			user, err = c.UserByEmail(ctx, email)
			if err != nil {
				return err
			}

			return c.setProviderForUserEmail(ctx, email, provider)
		}

		return err
	})
}

func (c *Cockroach) userByProvider(ctx context.Context, provider types.Provider) (types.User, error) {
	query := fmt.Sprintf(`SELECT %s FROM users WHERE %s_provider_id = @provider_id`, sqlUserCols, provider.Name)
	args := pgx.StrictNamedArgs{"provider_id": provider.ID}
	user, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowToStructByNameLax[types.User])
	if errors.Is(err, pgx.ErrNoRows) {
		return user, errs.NotFoundError("user not found")
	}

	if err != nil {
		return user, fmt.Errorf("sql select user by provider: %w", err)
	}

	return user, nil
}

func (c *Cockroach) UserProfileByUsername(ctx context.Context, in types.RetrieveUserProfile) (types.UserProfile, error) {
	var user types.UserProfile

	args := pgx.StrictNamedArgs{"username": in.Username}
	selects := []string{sqlUserProfileCols}
	joins := []string{}

	if in.ViewerID() != nil {
		args["viewer_id"] = *in.ViewerID()
		selects, joins = appendViewerRelationshipFields(selects, joins)
	}

	query := fmt.Sprintf(`
		SELECT %s
		FROM users
		%s
		WHERE users.username = @username
	`, strings.Join(selects, ",\n\t\t"), strings.Join(joins, "\n\t\t"))

	user, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowToStructByNameLax[types.UserProfile])
	if errors.Is(err, pgx.ErrNoRows) {
		return user, errs.NotFoundError("user not found")
	}

	if err != nil {
		return user, fmt.Errorf("sql select user profile by username: %w", err)
	}

	return user, nil
}

func (c *Cockroach) UserIDFromUsername(ctx context.Context, username string) (string, error) {
	const query = "SELECT id FROM users WHERE username = @username"
	args := pgx.StrictNamedArgs{"username": username}
	userID, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowTo[string])
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errs.NotFoundError("user not found")
	}

	if err != nil {
		return "", fmt.Errorf("sql select user ID from username: %w", err)
	}

	return userID, nil
}

func (c *Cockroach) EmailTaken(ctx context.Context, email, userID string) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1 FROM users WHERE email = @email AND id != @user_id
		)
	`
	args := pgx.StrictNamedArgs{
		"email":   email,
		"user_id": userID,
	}
	exists, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowTo[bool])
	if err != nil {
		return false, fmt.Errorf("sql select email taken: %w", err)
	}

	return exists, nil
}

func (c *Cockroach) UpdateUser(ctx context.Context, in types.UpdateUser) error {
	const query = `
		UPDATE users
		SET
			  username = COALESCE(@username, username)
			, bio = COALESCE(@bio, bio)
			, waifu = COALESCE(@waifu, waifu)
			, husbando = COALESCE(@husbando, husbando)
		WHERE id = @user_id
	`
	args := pgx.StrictNamedArgs{
		"username": in.Username,
		"bio":      in.Bio,
		"waifu":    in.Waifu,
		"husbando": in.Husbando,
		"user_id":  in.UserID(),
	}
	cmd, err := c.db.Exec(ctx, query, args)
	if db.IsUniqueViolationError(err, "username") {
		return errs.ConflictError("username taken")
	}

	if err != nil {
		return fmt.Errorf("sql update user: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return errs.NotFoundError("user not found")
	}

	return nil
}

func (c *Cockroach) UpdateEmail(ctx context.Context, userID, email string) (types.User, error) {
	const query = "UPDATE users SET email = @email WHERE id = @user_id RETURNING " + sqlUserCols
	args := pgx.StrictNamedArgs{
		"email":   email,
		"user_id": userID,
	}
	user, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowToStructByNameLax[types.User])
	if errors.Is(err, pgx.ErrNoRows) {
		return user, errs.NotFoundError("user not found")
	}

	if db.IsUniqueViolationError(err, "email") {
		return user, errs.ConflictError("email taken")
	}

	if err != nil {
		return user, fmt.Errorf("sql update email: %w", err)
	}

	return user, nil
}

func (c *Cockroach) UpdateAvatar(ctx context.Context, userID, avatar string) (*string, error) {
	const query = `
		WITH previous AS (
			SELECT avatar
			FROM users
			WHERE id = @user_id
		)
		UPDATE users
		SET avatar = @avatar
		FROM previous
		WHERE id = @user_id
		RETURNING previous.avatar
	`
	args := pgx.StrictNamedArgs{
		"avatar":  avatar,
		"user_id": userID,
	}
	oldAvatar, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowTo[*string])
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errs.NotFoundError("user not found")
	}

	if err != nil {
		return nil, fmt.Errorf("sql update avatar: %w", err)
	}

	return oldAvatar, nil
}

func (c *Cockroach) UpdateCover(ctx context.Context, userID, cover string) (*string, error) {
	const query = `
		WITH previous AS (
			SELECT cover
			FROM users
			WHERE id = @user_id
		)
		UPDATE users
		SET cover = @cover
		FROM previous
		WHERE id = @user_id
		RETURNING previous.cover
	`
	args := pgx.StrictNamedArgs{
		"cover":   cover,
		"user_id": userID,
	}
	oldCover, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowTo[*string])
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errs.NotFoundError("user not found")
	}

	if err != nil {
		return nil, fmt.Errorf("sql update cover: %w", err)
	}

	return oldCover, nil
}

func (c *Cockroach) setProviderForUserEmail(ctx context.Context, email string, provider types.Provider) error {
	query := fmt.Sprintf("UPDATE users SET %s_provider_id = @provider_id WHERE email = @email", provider.Name)
	args := pgx.StrictNamedArgs{
		"provider_id": provider.ID,
		"email":       email,
	}
	cmd, err := c.db.Exec(ctx, query, args)
	if err != nil {
		return fmt.Errorf("sql update user with provider id: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return errs.NotFoundError("user not found")
	}

	return nil
}

func (c *Cockroach) increaseFollowersCount(ctx context.Context, userID string) (uint, error) {
	const query = `
		UPDATE users
		SET followers_count = followers_count + 1
		WHERE id = @user_id
		RETURNING followers_count
	`
	args := pgx.StrictNamedArgs{"user_id": userID}
	count, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowTo[uint])
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, errs.NotFoundError("user not found")
	}

	if err != nil {
		return 0, fmt.Errorf("sql increase followers count: %w", err)
	}

	return count, nil
}

func (c *Cockroach) decreaseFollowersCount(ctx context.Context, userID string) (uint, error) {
	const query = `
		UPDATE users
		SET followers_count = followers_count - 1
		WHERE id = @user_id AND followers_count > 0
		RETURNING followers_count
	`
	args := pgx.StrictNamedArgs{"user_id": userID}
	count, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowTo[uint])
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, errs.NotFoundError("user not found")
	}

	if err != nil {
		return 0, fmt.Errorf("sql decrease followers count: %w", err)
	}

	return count, nil
}

func (c *Cockroach) increaseFolloweesCount(ctx context.Context, userID string) (uint, error) {
	const query = `
		UPDATE users
		SET followees_count = followees_count + 1
		WHERE id = @user_id
		RETURNING followees_count
	`
	args := pgx.StrictNamedArgs{"user_id": userID}
	count, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowTo[uint])
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, errs.NotFoundError("user not found")
	}

	if err != nil {
		return 0, fmt.Errorf("sql increase followees count: %w", err)
	}

	return count, nil
}

func (c *Cockroach) decreaseFolloweesCount(ctx context.Context, userID string) (uint, error) {
	const query = `
		UPDATE users
		SET followees_count = followees_count - 1
		WHERE id = @user_id AND followees_count > 0
		RETURNING followees_count
	`
	args := pgx.StrictNamedArgs{"user_id": userID}
	count, err := pgxutil.SelectRow(ctx, c.db, query, []any{args}, pgx.RowTo[uint])
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, errs.NotFoundError("user not found")
	}

	if err != nil {
		return 0, fmt.Errorf("sql decrease followees count: %w", err)
	}

	return count, nil
}

func userProfileCursor(u types.UserProfile) Cursor[any] {
	return Cursor[any]{ID: u.Username}
}
