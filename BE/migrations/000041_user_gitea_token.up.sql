-- Per-user Gitea PAT so comments are posted on behalf of each user instead of
-- the workspace integration token. Stored encrypted (same scheme as
-- gitea_instances.access_token).
ALTER TABLE users ADD COLUMN IF NOT EXISTS gitea_token TEXT;
