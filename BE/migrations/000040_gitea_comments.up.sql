ALTER TABLE users ADD COLUMN IF NOT EXISTS gitea_login VARCHAR(100);
CREATE INDEX IF NOT EXISTS idx_users_gitea_login ON users (LOWER(gitea_login)) WHERE gitea_login IS NOT NULL;

ALTER TABLE comments ALTER COLUMN user_id DROP NOT NULL;
ALTER TABLE comments ADD COLUMN IF NOT EXISTS author_login VARCHAR(100);
ALTER TABLE comments ADD COLUMN IF NOT EXISTS author_avatar_url TEXT;
ALTER TABLE comments ADD COLUMN IF NOT EXISTS gitea_comment_id BIGINT;
CREATE INDEX IF NOT EXISTS idx_comments_gitea_comment ON comments (gitea_comment_id) WHERE gitea_comment_id IS NOT NULL;
