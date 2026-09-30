-- Revert statuses from workspace level back to team level.
-- Statuses created while statuses were workspace-scoped (team_id IS NULL) are
-- re-attached to the first team of their workspace; the rest are removed.

DROP INDEX IF EXISTS idx_team_statuses_workspace;
DROP INDEX IF EXISTS idx_team_statuses_workspace_slug;

UPDATE team_statuses ts
SET team_id = (
    SELECT t.id FROM teams t
    WHERE t.workspace_id = ts.workspace_id
    ORDER BY t.created_at, t.id
    LIMIT 1
)
WHERE ts.team_id IS NULL AND ts.workspace_id IS NOT NULL;

DELETE FROM team_statuses WHERE team_id IS NULL;

ALTER TABLE team_statuses ALTER COLUMN team_id SET NOT NULL;
ALTER TABLE team_statuses DROP COLUMN workspace_id;
