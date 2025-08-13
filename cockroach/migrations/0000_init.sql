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
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW() ON UPDATE NOW()
);

ALTER TABLE users ADD COLUMN IF NOT EXISTS avatar JSONB;

CREATE UNIQUE INDEX IF NOT EXISTS users_email_idx ON users (email);
CREATE UNIQUE INDEX IF NOT EXISTS users_username_idx ON users (username);

CREATE TABLE IF NOT EXISTS posts (
    id VARCHAR NOT NULL PRIMARY KEY,
    user_id VARCHAR NOT NULL REFERENCES users ON DELETE CASCADE ON UPDATE CASCADE,
    content TEXT NOT NULL,
    is_r18 BOOLEAN NOT NULL DEFAULT FALSE,
    attachments JSONB,
    -- comments_count INT NOT NULL DEFAULT 0, -- updated by [comments_count_trigger]
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW() ON UPDATE NOW()
);

CREATE INDEX IF NOT EXISTS posts_user_id_idx ON posts (user_id);

CREATE TABLE IF NOT EXISTS comments (
    id VARCHAR NOT NULL PRIMARY KEY,
    user_id VARCHAR NOT NULL REFERENCES users ON DELETE CASCADE ON UPDATE CASCADE,
    post_id VARCHAR NOT NULL REFERENCES posts ON DELETE CASCADE ON UPDATE CASCADE,
    content TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW() ON UPDATE NOW()
);

CREATE INDEX IF NOT EXISTS comments_user_id_idx ON comments (user_id);
CREATE INDEX IF NOT EXISTS comments_post_id_idx ON comments (post_id);

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

CREATE TABLE IF NOT EXISTS follows (
    follower_id VARCHAR NOT NULL REFERENCES users ON DELETE CASCADE ON UPDATE CASCADE,
    followee_id VARCHAR NOT NULL REFERENCES users ON DELETE CASCADE ON UPDATE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (follower_id, followee_id)
);

CREATE INDEX IF NOT EXISTS follows_follower_id_idx ON follows (follower_id);
CREATE INDEX IF NOT EXISTS follows_followee_id_idx ON follows (followee_id);

-- CREATE OR REPLACE FUNCTION update_comments_count()
-- RETURNS TRIGGER AS $$
-- DECLARE
--     post_id_to_update VARCHAR;
-- BEGIN
--     IF TG_OP = 'INSERT' THEN
--         post_id_to_update := NEW.post_id;
--     ELSIF TG_OP = 'DELETE' THEN
--         post_id_to_update := OLD.post_id;
--     END IF;

--     UPDATE posts 
--     SET comments_count = (
--         SELECT COUNT(*) 
--         FROM comments 
--         WHERE comments.post_id = post_id_to_update
--     )
--     WHERE id = post_id_to_update;

--     IF TG_OP = 'INSERT' THEN
--         RETURN NEW;
--     ELSE
--         RETURN OLD;
--     END IF;
-- END;
-- $$ LANGUAGE plpgsql;

-- DROP TRIGGER IF EXISTS comments_count_trigger ON comments;
-- CREATE TRIGGER comments_count_trigger
--     AFTER INSERT OR DELETE ON comments
--     FOR EACH ROW EXECUTE FUNCTION update_comments_count();

CREATE TABLE IF NOT EXISTS notifications (
    id VARCHAR NOT NULL PRIMARY KEY,
    user_id VARCHAR NOT NULL REFERENCES users ON DELETE CASCADE ON UPDATE CASCADE,
    kind VARCHAR NOT NULL,
    actor_user_ids VARCHAR[] NOT NULL, -- keeps only the latest few, full list is on [notification_actors] table.
    actors_count INT NOT NULL DEFAULT 1, -- stores the full total count.
    notifiable_kind VARCHAR, -- kind of the entity being notified (e.g., post, comment, publication, chapter)
    notifiable_id VARCHAR, -- ID of the entity being notified (e.g., post_id, comment_id, publication_id, chapter_id)
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

CREATE TABLE IF NOT EXISTS feed (
    user_id VARCHAR NOT NULL REFERENCES users ON DELETE CASCADE ON UPDATE CASCADE,
    post_id VARCHAR NOT NULL REFERENCES posts ON DELETE CASCADE ON UPDATE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, post_id)
);

CREATE INDEX IF NOT EXISTS feed_user_id_idx ON feed (user_id);