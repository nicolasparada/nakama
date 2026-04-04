-- CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- DROP DATABASE IF EXISTS nakama CASCADE;
CREATE DATABASE IF NOT EXISTS nakama;
SET DATABASE = nakama;

CREATE TABLE IF NOT EXISTS users (
    id UUID NOT NULL PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR NOT NULL UNIQUE,
    username VARCHAR NOT NULL UNIQUE,
    avatar VARCHAR,
    google_provider_id VARCHAR UNIQUE,
    github_provider_id VARCHAR UNIQUE,
    cover VARCHAR,
    bio VARCHAR,
    waifu VARCHAR,
    husbando VARCHAR,
    followers_count INT NOT NULL DEFAULT 0 CHECK (followers_count >= 0),
    followees_count INT NOT NULL DEFAULT 0 CHECK (followees_count >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS email_verification_codes (
    user_id UUID REFERENCES users ON DELETE CASCADE,
    email VARCHAR NOT NULL,
    code UUID NOT NULL DEFAULT gen_random_uuid(),
    redirect_uri VARCHAR NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (email, code)
);

ALTER TABLE IF EXISTS email_verification_codes ADD COLUMN IF NOT EXISTS user_id UUID REFERENCES users ON DELETE CASCADE;
ALTER TABLE IF EXISTS email_verification_codes ADD COLUMN IF NOT EXISTS redirect_uri VARCHAR NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS follows (
    follower_id UUID NOT NULL REFERENCES users ON DELETE CASCADE,
    followee_id UUID NOT NULL REFERENCES users ON DELETE CASCADE,
    PRIMARY KEY (follower_id, followee_id)
);

CREATE TABLE IF NOT EXISTS posts (
    id UUID NOT NULL PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users ON DELETE CASCADE,
    content VARCHAR NOT NULL,
    media VARCHAR[],
    spoiler_of VARCHAR,
    nsfw BOOLEAN NOT NULL DEFAULT false,
    reactions JSONB,
    comments_count INT NOT NULL DEFAULT 0 CHECK (comments_count >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMP NOT NULL DEFAULT now(),
    INDEX sorted_posts (created_at DESC, id)
);

ALTER TABLE posts ADD COLUMN media_jsonb JSONB;

UPDATE posts
SET media_jsonb = CASE
    WHEN media IS NULL THEN NULL
    ELSE COALESCE(
        (
            SELECT jsonb_agg(jsonb_build_object('path', path))
            FROM unnest(media) AS t(path)
        ),
        '[]'::JSONB
    )
END;

ALTER TABLE posts DROP COLUMN media;
ALTER TABLE posts RENAME COLUMN media_jsonb TO media;

CREATE TABLE IF NOT EXISTS post_reactions (
    user_id UUID NOT NULL REFERENCES users ON DELETE CASCADE,
    post_id UUID NOT NULL REFERENCES posts ON DELETE CASCADE,
    reaction VARCHAR NOT NULL,
    kind VARCHAR NOT NULL,
    PRIMARY KEY (user_id, post_id, reaction)
);

ALTER TABLE post_reactions 
ADD CONSTRAINT IF NOT EXISTS post_reactions_kind_check 
CHECK (kind IN ('emoji', 'custom'));

CREATE INDEX IF NOT EXISTS idx_post_reactions_post_user
ON post_reactions (post_id, user_id)
STORING (kind);

CREATE TABLE IF NOT EXISTS post_subscriptions (
    user_id UUID NOT NULL REFERENCES users ON DELETE CASCADE,
    post_id UUID NOT NULL REFERENCES posts ON DELETE CASCADE,
    PRIMARY KEY (user_id, post_id)
);

CREATE TABLE IF NOT EXISTS timeline (
    id UUID NOT NULL PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users ON DELETE CASCADE,
    post_id UUID NOT NULL REFERENCES posts ON DELETE CASCADE,
    UNIQUE INDEX unique_timeline_items (user_id, post_id)
);

CREATE TABLE IF NOT EXISTS comments (
    id UUID NOT NULL PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users ON DELETE CASCADE,
    post_id UUID NOT NULL REFERENCES posts ON DELETE CASCADE,
    content VARCHAR NOT NULL,
    reactions JSONB, -- [{ "kind": "emoji", "reaction": "❤️", "count": 3 }]
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_comments_post_id_sorted
ON comments (post_id, created_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS comment_reactions (
    user_id UUID NOT NULL REFERENCES users ON DELETE CASCADE,
    comment_id UUID NOT NULL REFERENCES comments ON DELETE CASCADE,
    reaction VARCHAR NOT NULL,
    kind VARCHAR NOT NULL,
    PRIMARY KEY (user_id, comment_id, reaction)
);

ALTER TABLE comment_reactions 
ADD CONSTRAINT IF NOT EXISTS comment_reactions_kind_check 
CHECK (kind IN ('emoji', 'custom'));

CREATE INDEX IF NOT EXISTS idx_comment_reactions_comment_user
ON comment_reactions (comment_id, user_id)
STORING (kind);

CREATE TABLE IF NOT EXISTS post_tags (
    id UUID NOT NULL PRIMARY KEY DEFAULT gen_random_uuid(),
    post_id UUID NOT NULL REFERENCES posts ON DELETE CASCADE,
    comment_id UUID REFERENCES comments ON DELETE CASCADE,
    tag VARCHAR NOT NULL
);

CREATE TABLE IF NOT EXISTS notifications (
    id UUID NOT NULL PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users ON DELETE CASCADE,
    actor_usernames VARCHAR[] NOT NULL,
    kind VARCHAR NOT NULL,
    post_id UUID REFERENCES posts ON DELETE CASCADE,
    comment_id UUID REFERENCES comments ON DELETE CASCADE,
    read_at TIMESTAMPTZ,
    issued_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    INDEX sorted_notifications (issued_at DESC, id)
);

DROP INDEX IF EXISTS unique_notifications;

CREATE UNIQUE INDEX IF NOT EXISTS unique_comment_unread_notifications
ON notifications (user_id, kind, post_id)
WHERE kind = 'comment' AND read_at IS NULL;

ALTER TABLE notifications ADD COLUMN IF NOT EXISTS actor_user_ids UUID[] NOT NULL DEFAULT '{}'; -- only the last 2 actors. Used for showing: user_a and user_b did something.
ALTER TABLE notifications ADD COLUMN IF NOT EXISTS actors_count INT NOT NULL DEFAULT 0; -- total count used for showing: user_a and 3 others did something.

CREATE TABLE IF NOT EXISTS notification_actors (
    user_id UUID NOT NULL REFERENCES users ON DELETE CASCADE,
    notification_id UUID NOT NULL REFERENCES notifications ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, notification_id)
);

DROP TRIGGER IF EXISTS notification_actors_update_trigger ON notification_actors;

CREATE OR REPLACE FUNCTION update_notification_actors()
RETURNS TRIGGER AS $$
DECLARE
    notification_id_to_update UUID;
    latest_two_actor_user_ids UUID[];
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

ALTER TABLE notifications DROP COLUMN IF EXISTS actor_usernames;

CREATE TABLE IF NOT EXISTS user_web_push_subscriptions (
    id UUID NOT NULL PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users ON DELETE CASCADE,
    sub JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE INDEX unique_user_web_push_subscriptions (user_id, (sub->>'endpoint'))
);

-- INSERT INTO users (id, email, username) VALUES
--     ('504c9492-bde3-4b86-862a-e2fbb6ea0363', 'shinji@example.org', 'shinji'),
--     ('cc51e41c-f18c-43e2-a172-32a06faad175', 'rei@example.org', 'rei'),
--     ('ed354685-3490-4887-92df-2905bb358915', 'asuka@example.org', 'asuka'),
--     ('ca5d2248-93c9-4a5b-ba1a-1474ed2939a9', 'misato@example.org', 'misato'),
--     ('17523b13-acf5-4cc9-9b17-202bbbb14635', 'ritsuko@example.org', 'ritsuko'),
--     ('75251bef-d175-4e47-b024-7d2c3d4d123f', 'gendo@example.org', 'gendo');