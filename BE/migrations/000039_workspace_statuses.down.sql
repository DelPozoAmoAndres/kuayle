-- Revert statuses from workspace level back to team level.
-- Statuses created while statuses were workspace-scoped (team_id IS NULL) are
-- either re-attached to the first team of their workspace or, when that team
-- already owns an equivalent status (UNIQUE(team_id, slug)), their references
-- are repointed to it and the duplicate is removed.

DROP INDEX IF EXISTS idx_team_statuses_workspace;
DROP INDEX IF EXISTS idx_team_statuses_workspace_slug;

-- 1. The previous schema required every workspace to own a team: recreate one
--    for workspaces that still have workspace-level statuses but no team.
INSERT INTO teams (id, workspace_id, name, key)
SELECT gen_random_uuid(), w.id, 'Default', 'DEF'
FROM workspaces w
WHERE NOT EXISTS (SELECT 1 FROM teams t WHERE t.workspace_id = w.id)
  AND EXISTS (
      SELECT 1 FROM team_statuses ts
      WHERE ts.workspace_id = w.id AND ts.team_id IS NULL
  );

-- 2. Repoint every reference to the team-owned status with the same slug.
--    (project_status_visibility cascades, auto-transitions fall back to NULL.)
WITH first_team AS (
    SELECT DISTINCT ON (t.workspace_id) t.workspace_id, t.id AS team_id
    FROM teams t
    ORDER BY t.workspace_id, t.created_at, t.id
)
UPDATE issues i
SET status_id = ts.id
FROM team_statuses ws
JOIN first_team ft ON ft.workspace_id = ws.workspace_id
JOIN team_statuses ts ON ts.team_id = ft.team_id AND ts.slug = ws.slug
WHERE i.status_id = ws.id
  AND ws.team_id IS NULL;

WITH first_team AS (
    SELECT DISTINCT ON (t.workspace_id) t.workspace_id, t.id AS team_id
    FROM teams t
    ORDER BY t.workspace_id, t.created_at, t.id
)
UPDATE github_auto_transitions gt
SET target_status_id = ts.id
FROM team_statuses ws
JOIN first_team ft ON ft.workspace_id = ws.workspace_id
JOIN team_statuses ts ON ts.team_id = ft.team_id AND ts.slug = ws.slug
WHERE gt.target_status_id = ws.id
  AND ws.team_id IS NULL;

WITH first_team AS (
    SELECT DISTINCT ON (t.workspace_id) t.workspace_id, t.id AS team_id
    FROM teams t
    ORDER BY t.workspace_id, t.created_at, t.id
)
UPDATE gitea_auto_transitions gt
SET target_status_id = ts.id
FROM team_statuses ws
JOIN first_team ft ON ft.workspace_id = ws.workspace_id
JOIN team_statuses ts ON ts.team_id = ft.team_id AND ts.slug = ws.slug
WHERE gt.target_status_id = ws.id
  AND ws.team_id IS NULL;

-- 3. Attach the workspace-level statuses whose slug is still free on the team.
WITH first_team AS (
    SELECT DISTINCT ON (t.workspace_id) t.workspace_id, t.id AS team_id
    FROM teams t
    ORDER BY t.workspace_id, t.created_at, t.id
)
UPDATE team_statuses ws
SET team_id = ft.team_id
FROM first_team ft
WHERE ws.team_id IS NULL
  AND ws.workspace_id = ft.workspace_id
  AND NOT EXISTS (
      SELECT 1 FROM team_statuses other
      WHERE other.team_id = ft.team_id AND other.slug = ws.slug
  );

-- 4. The rest were duplicates of an existing team status: their references
--    were repointed in step 2, so nothing points at them anymore.
DELETE FROM team_statuses ws
WHERE ws.team_id IS NULL
  AND NOT EXISTS (SELECT 1 FROM issues i WHERE i.status_id = ws.id);

-- 5. Every team must end up with the whole default set again (the
--    deduplication left some teams with only a few of them).
INSERT INTO team_statuses (team_id, name, slug, category, position, is_default)
SELECT t.id, s.name, s.slug, s.category, s.position, s.is_default
FROM teams t
CROSS JOIN (VALUES
    ('Backlog',     'backlog',     'backlog',   0, true),
    ('Todo',        'todo',        'unstarted', 1, false),
    ('In Progress', 'in_progress', 'started',   2, false),
    ('In Review',   'in_review',   'started',   3, false),
    ('Done',        'done',        'completed', 4, false),
    ('Cancelled',   'cancelled',   'cancelled', 5, false)
) AS s(name, slug, category, position, is_default)
WHERE NOT EXISTS (
    SELECT 1 FROM team_statuses ts WHERE ts.team_id = t.id AND ts.slug = s.slug
);

-- 6. Move issues back onto the status owned by their own team.
UPDATE issues i
SET status_id = own.id
FROM team_statuses other
JOIN team_statuses own ON own.slug = other.slug
WHERE i.status_id = other.id
  AND other.team_id IS DISTINCT FROM i.team_id
  AND own.team_id = i.team_id;

-- 7. Restore the previous NOT NULL contract.
ALTER TABLE team_statuses ALTER COLUMN team_id SET NOT NULL;
ALTER TABLE team_statuses DROP COLUMN workspace_id;
