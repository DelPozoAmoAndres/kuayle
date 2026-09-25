-- Restore team_id NOT NULL constraint (only safe if no NULLs exist)
UPDATE issues SET team_id = (SELECT id FROM teams LIMIT 1) WHERE team_id IS NULL;

ALTER TABLE issues ALTER COLUMN team_id SET NOT NULL;

-- Restore the original unique constraint
DROP INDEX IF EXISTS idx_issues_team_number;
ALTER TABLE issues ADD CONSTRAINT issues_team_id_number_key UNIQUE (team_id, number);

DROP INDEX IF EXISTS idx_issues_team_status;
CREATE INDEX idx_issues_team_status ON issues(team_id, status);
