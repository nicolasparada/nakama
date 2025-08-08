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

CREATE UNIQUE INDEX IF NOT EXISTS users_email_idx ON users (email);
CREATE UNIQUE INDEX IF NOT EXISTS users_username_idx ON users (username);

CREATE TABLE IF NOT EXISTS posts (
    id VARCHAR NOT NULL PRIMARY KEY,
    user_id VARCHAR NOT NULL REFERENCES users ON DELETE CASCADE ON UPDATE CASCADE,
    content TEXT NOT NULL,
    attachments JSONB,
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
