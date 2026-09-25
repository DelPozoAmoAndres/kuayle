ALTER TABLE issues ADD COLUMN IF NOT EXISTS gitea_issue_index BIGINT;
ALTER TABLE issues ADD COLUMN IF NOT EXISTS gitea_instance_id UUID REFERENCES gitea_instances(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_issues_gitea ON issues(gitea_instance_id, gitea_issue_index);
