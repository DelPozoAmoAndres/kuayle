DROP INDEX IF EXISTS idx_issues_gitea;
ALTER TABLE issues DROP COLUMN IF EXISTS gitea_instance_id;
ALTER TABLE issues DROP COLUMN IF EXISTS gitea_issue_index;
