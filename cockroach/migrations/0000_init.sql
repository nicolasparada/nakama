CREATE TABLE IF NOT EXISTS sessions (
	token TEXT PRIMARY KEY,
	data BYTEA NOT NULL,
	expiry TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS sessions_expiry_idx ON sessions (expiry);

CREATE TABLE IF NOT EXISTS users (
    id VARCHAR NOT NULL PRIMARY KEY,
    email VARCHAR NOT NULL,
    username VARCHAR NOT NULL,
    avatar JSONB,
    followers_count INT NOT NULL DEFAULT 0, -- updated by [follows_count_trigger]
    following_count INT NOT NULL DEFAULT 0, -- updated by [follows_count_trigger]
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW() ON UPDATE NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS users_email_idx ON users (email);
CREATE UNIQUE INDEX IF NOT EXISTS users_username_idx ON users (username);
CREATE INDEX IF NOT EXISTS users_username_trgm_idx ON users USING GIN(username gin_trgm_ops);

CREATE TABLE IF NOT EXISTS follows (
    follower_id VARCHAR NOT NULL REFERENCES users ON DELETE CASCADE ON UPDATE CASCADE,
    followee_id VARCHAR NOT NULL REFERENCES users ON DELETE CASCADE ON UPDATE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (follower_id, followee_id)
);

CREATE INDEX IF NOT EXISTS follows_follower_id_idx ON follows (follower_id);
CREATE INDEX IF NOT EXISTS follows_followee_id_idx ON follows (followee_id);

DROP TRIGGER IF EXISTS follows_count_trigger ON follows;

CREATE OR REPLACE FUNCTION update_follows_count()
RETURNS TRIGGER AS $$
DECLARE
    follower_user_id VARCHAR;
    followee_user_id VARCHAR;
BEGIN
    IF TG_OP = 'INSERT' THEN
        follower_user_id := (NEW).follower_id;
        followee_user_id := (NEW).followee_id;
    ELSIF TG_OP = 'DELETE' THEN
        follower_user_id := (OLD).follower_id;
        followee_user_id := (OLD).followee_id;
    END IF;

    UPDATE users 
    SET following_count = (
        SELECT COUNT(*) 
        FROM follows 
        WHERE follows.follower_id = follower_user_id
    )
    WHERE id = follower_user_id;

    UPDATE users 
    SET followers_count = (
        SELECT COUNT(*) 
        FROM follows 
        WHERE follows.followee_id = followee_user_id
    )
    WHERE id = followee_user_id;

    IF TG_OP = 'INSERT' THEN
        RETURN (NEW);
    ELSE
        RETURN (OLD);
    END IF;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER follows_count_trigger
    AFTER INSERT OR DELETE ON follows
    FOR EACH ROW EXECUTE FUNCTION update_follows_count();

CREATE TABLE IF NOT EXISTS posts (
    id VARCHAR NOT NULL PRIMARY KEY,
    user_id VARCHAR NOT NULL REFERENCES users ON DELETE CASCADE ON UPDATE CASCADE,
    content TEXT NOT NULL, -- Can be empty string, not nullable
    is_r18 BOOLEAN NOT NULL DEFAULT FALSE,
    attachments JSONB,
    reactions_summary JSONB, -- updated by [reactions_update_trigger]
    comments_count INT NOT NULL DEFAULT 0, -- updated by [comments_count_trigger]
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW() ON UPDATE NOW()
);

CREATE INDEX IF NOT EXISTS posts_user_id_idx ON posts (user_id);
CREATE INDEX IF NOT EXISTS posts_content_trgm_idx ON posts USING GIN(content gin_trgm_ops);

CREATE TABLE IF NOT EXISTS comments (
    id VARCHAR NOT NULL PRIMARY KEY,
    user_id VARCHAR NOT NULL REFERENCES users ON DELETE CASCADE ON UPDATE CASCADE,
    post_id VARCHAR NOT NULL REFERENCES posts ON DELETE CASCADE ON UPDATE CASCADE,
    content TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW() ON UPDATE NOW()
);

DROP TRIGGER IF EXISTS comments_count_trigger ON comments;

CREATE OR REPLACE FUNCTION update_comments_count()
RETURNS TRIGGER AS $$
DECLARE
    post_id_to_update VARCHAR;
BEGIN
    IF TG_OP = 'INSERT' THEN
        post_id_to_update := (NEW).post_id;
    ELSIF TG_OP = 'DELETE' THEN
        post_id_to_update := (OLD).post_id;
    END IF;

    UPDATE posts 
    SET comments_count = (
        SELECT COUNT(*) 
        FROM comments 
        WHERE comments.post_id = post_id_to_update
    )
    WHERE id = post_id_to_update;

    IF TG_OP = 'INSERT' THEN
        RETURN (NEW);
    ELSE
        RETURN (OLD);
    END IF;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER comments_count_trigger
    AFTER INSERT OR DELETE ON comments
    FOR EACH ROW EXECUTE FUNCTION update_comments_count();

CREATE INDEX IF NOT EXISTS comments_user_id_idx ON comments (user_id);
CREATE INDEX IF NOT EXISTS comments_post_id_idx ON comments (post_id);

CREATE TABLE IF NOT EXISTS reactions (
    user_id VARCHAR NOT NULL REFERENCES users ON DELETE CASCADE ON UPDATE CASCADE,
    post_id VARCHAR NOT NULL REFERENCES posts ON DELETE CASCADE ON UPDATE CASCADE,
    emoji VARCHAR NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, post_id, emoji)
);

CREATE INDEX IF NOT EXISTS reactions_post_id_idx ON reactions (post_id);
CREATE INDEX IF NOT EXISTS reactions_user_id_idx ON reactions (user_id);
CREATE INDEX IF NOT EXISTS reactions_emoji_idx ON reactions (emoji);

DROP TRIGGER IF EXISTS reactions_update_trigger ON reactions;

CREATE OR REPLACE FUNCTION update_post_reactions()
RETURNS TRIGGER AS $$
DECLARE
    target_post_id VARCHAR;
    reaction_summary JSONB;
    reaction_count INTEGER;
BEGIN
    IF TG_OP = 'INSERT' THEN
        target_post_id := (NEW).post_id;
    ELSIF TG_OP = 'DELETE' THEN
        target_post_id := (OLD).post_id;
    END IF;

    SELECT COUNT(*) INTO reaction_count
    FROM reactions 
    WHERE post_id = target_post_id;

    IF reaction_count = 0 THEN
        reaction_summary := '[]'::jsonb;
    ELSE
        SELECT jsonb_agg(
            jsonb_build_object(
                'emoji', emoji,
                'count', count
            ) ORDER BY count DESC, emoji ASC
        )
        INTO reaction_summary
        FROM (
            SELECT emoji, COUNT(*)::int as count
            FROM reactions 
            WHERE post_id = target_post_id
            GROUP BY emoji
        ) reaction_counts;
        
        IF reaction_summary IS NULL THEN
            reaction_summary := '[]'::jsonb;
        END IF;
    END IF;

    UPDATE posts 
    SET reactions_summary = reaction_summary
    WHERE id = target_post_id;

    IF TG_OP = 'INSERT' THEN
        RETURN (NEW);
    ELSE
        RETURN (OLD);
    END IF;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER reactions_update_trigger
    AFTER INSERT OR DELETE ON reactions
    FOR EACH ROW EXECUTE FUNCTION update_post_reactions();

CREATE TABLE IF NOT EXISTS publications (
    id VARCHAR NOT NULL PRIMARY KEY,
    user_id VARCHAR NOT NULL REFERENCES users ON DELETE CASCADE ON UPDATE CASCADE,
    kind VARCHAR NOT NULL,
    title VARCHAR NOT NULL,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW() ON UPDATE NOW()
);

CREATE INDEX IF NOT EXISTS publications_user_id_idx ON publications (user_id);
CREATE INDEX IF NOT EXISTS publications_kind_idx ON publications (kind);

CREATE TABLE IF NOT EXISTS chapters (
    id VARCHAR NOT NULL PRIMARY KEY,
    publication_id VARCHAR NOT NULL REFERENCES publications ON DELETE CASCADE ON UPDATE CASCADE,
    title VARCHAR NOT NULL,
    content TEXT NOT NULL,
    number INT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW() ON UPDATE NOW()
);

CREATE INDEX IF NOT EXISTS chapters_publication_id_idx ON chapters (publication_id);
CREATE UNIQUE INDEX IF NOT EXISTS chapters_publication_id_number_idx ON chapters (publication_id, number);

CREATE TABLE IF NOT EXISTS notifications (
    id VARCHAR NOT NULL PRIMARY KEY,
    user_id VARCHAR NOT NULL REFERENCES users ON DELETE CASCADE ON UPDATE CASCADE,
    kind VARCHAR NOT NULL,
    actor_user_ids VARCHAR[] NOT NULL, -- only the latest 2 actors max. Updated by [update_notification_actors]
    actors_count INT NOT NULL DEFAULT 1, -- updated by [update_notification_actors]
    notifiable_kind VARCHAR, -- post, comment, publication, chapter
    notifiable_id VARCHAR, -- post_id, comment_id, publication_id, chapter_id
    read_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW() ON UPDATE NOW()
);

CREATE INDEX IF NOT EXISTS notifications_user_id_idx ON notifications (user_id);
CREATE INDEX IF NOT EXISTS notifications_user_id_read_at_idx ON notifications (user_id, read_at);
CREATE UNIQUE INDEX IF NOT EXISTS notifications_user_id_kind_notifiable_kind_notifiable_id_idx ON notifications (user_id, kind, notifiable_kind, notifiable_id, read_at);

CREATE TABLE IF NOT EXISTS notification_actors (
    user_id VARCHAR NOT NULL REFERENCES users ON DELETE CASCADE ON UPDATE CASCADE,
    notification_id VARCHAR NOT NULL REFERENCES notifications ON DELETE CASCADE ON UPDATE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW() ON UPDATE NOW(),
    PRIMARY KEY (user_id, notification_id)
);

CREATE INDEX IF NOT EXISTS notification_actors_user_id_idx ON notification_actors (user_id);
CREATE INDEX IF NOT EXISTS notification_actors_notification_id_idx ON notification_actors (notification_id);

DROP TRIGGER IF EXISTS notification_actors_update_trigger ON notification_actors;

CREATE OR REPLACE FUNCTION update_notification_actors()
RETURNS TRIGGER AS $$
DECLARE
    notification_id_to_update VARCHAR;
    latest_two_actor_user_ids VARCHAR[];
    total_count INT;
BEGIN
    IF TG_OP = 'INSERT' THEN
        notification_id_to_update := (NEW).notification_id;
    ELSIF TG_OP = 'DELETE' THEN
        notification_id_to_update := (OLD).notification_id;
    END IF;

    SELECT COUNT(*) INTO total_count
    FROM notification_actors 
    WHERE notification_id = notification_id_to_update;

    SELECT ARRAY(
        SELECT user_id 
        FROM notification_actors 
        WHERE notification_id = notification_id_to_update
        ORDER BY created_at DESC
        LIMIT 2
    ) INTO latest_two_actor_user_ids;

    UPDATE notifications 
    SET 
        actor_user_ids = latest_two_actor_user_ids,
        actors_count = total_count
    WHERE id = notification_id_to_update;

    IF TG_OP = 'INSERT' THEN
        RETURN (NEW);
    ELSE
        RETURN (OLD);
    END IF;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER notification_actors_update_trigger
    AFTER INSERT OR DELETE ON notification_actors
    FOR EACH ROW EXECUTE FUNCTION update_notification_actors();

CREATE TABLE IF NOT EXISTS feed (
    user_id VARCHAR NOT NULL REFERENCES users ON DELETE CASCADE ON UPDATE CASCADE,
    post_id VARCHAR NOT NULL REFERENCES posts ON DELETE CASCADE ON UPDATE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, post_id)
);

CREATE INDEX IF NOT EXISTS feed_user_id_idx ON feed (user_id);
