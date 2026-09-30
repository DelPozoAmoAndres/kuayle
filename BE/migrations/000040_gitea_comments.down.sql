DROP INDEX IF EXISTS idx_comments_gitea_comment;
ALTER TABLE comments DROP COLUMN IF EXISTS gitea_comment_id;
ALTER TABLE comments DROP COLUMN IF EXISTS author_avatar_url;
ALTER TABLE comments DROP COLUMN IF EXISTS author_login;
DELETE FROM comments WHERE user_id IS NULL;
ALTER TABLE comments ALTER COLUMN user_id SET NOT NULL;

DROP INDEX IF EXISTS idx_users_gitea_login;
ALTER TABLE users DROP COLUMN IF EXISTS gitea_login;
