-- Allow issues without a team
ALTER TABLE issues ALTER COLUMN team_id DROP NOT NULL;

-- Drop the unique constraint on (team_id, number) since NULLs don't participate in UNIQUE.
-- Replace with a partial unique index that only enforces uniqueness for non-NULL team_ids.
DROP INDEX IF EXISTS idx_issues_team_status;
ALTER TABLE issues DROP CONSTRAINT IF EXISTS issues_team_id_number_key;
CREATE UNIQUE INDEX idx_issues_team_number ON issues(team_id, number) WHERE team_id IS NOT NULL;
CREATE INDEX idx_issues_team_status ON issues(team_id, status);
