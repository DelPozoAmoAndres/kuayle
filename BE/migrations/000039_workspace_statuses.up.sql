-- Move statuses from per-team to per-workspace.
-- The table keeps its legacy name `team_statuses` because several PL/pgSQL
-- triggers reference it; only the ownership column changes.

-- 1. Add the workspace ownership column
ALTER TABLE team_statuses ADD COLUMN IF NOT EXISTS workspace_id UUID REFERENCES workspaces(id);

-- 2. Backfill workspace_id from the parent team
UPDATE team_statuses ts
SET workspace_id = t.workspace_id
FROM teams t
WHERE ts.team_id = t.id AND ts.workspace_id IS NULL;

-- 3. Statuses no longer belong to a team. This must happen BEFORE seeding
--    workspace-level statuses, which are inserted with team_id = NULL
--    (team_id stays as a legacy nullable column).
ALTER TABLE team_statuses ALTER COLUMN team_id DROP NOT NULL;

-- 4. Deduplicate by (workspace_id, slug) keeping the lowest position/id.
--    Before deleting a duplicate, repoint every FK to the surviving row.
WITH ranked AS (
    SELECT id, workspace_id, slug,
           ROW_NUMBER() OVER (PARTITION BY workspace_id, slug ORDER BY position, id) AS rn
    FROM team_statuses
    WHERE workspace_id IS NOT NULL
),
dups AS (
    SELECT d.id AS dup_id, k.id AS survivor_id
    FROM ranked d
    INNER JOIN ranked k
        ON k.workspace_id = d.workspace_id AND k.slug = d.slug AND k.rn = 1
    WHERE d.rn > 1
)
UPDATE issues i
SET status_id = dups.survivor_id
FROM dups
WHERE i.status_id = dups.dup_id;

WITH ranked AS (
    SELECT id, workspace_id, slug,
           ROW_NUMBER() OVER (PARTITION BY workspace_id, slug ORDER BY position, id) AS rn
    FROM team_statuses
    WHERE workspace_id IS NOT NULL
),
dups AS (
    SELECT d.id AS dup_id, k.id AS survivor_id
    FROM ranked d
    INNER JOIN ranked k
        ON k.workspace_id = d.workspace_id AND k.slug = d.slug AND k.rn = 1
    WHERE d.rn > 1
)
UPDATE github_auto_transitions gt
SET target_status_id = dups.survivor_id
FROM dups
WHERE gt.target_status_id = dups.dup_id;

WITH ranked AS (
    SELECT id, workspace_id, slug,
           ROW_NUMBER() OVER (PARTITION BY workspace_id, slug ORDER BY position, id) AS rn
    FROM team_statuses
    WHERE workspace_id IS NOT NULL
),
dups AS (
    SELECT d.id AS dup_id, k.id AS survivor_id
    FROM ranked d
    INNER JOIN ranked k
        ON k.workspace_id = d.workspace_id AND k.slug = d.slug AND k.rn = 1
    WHERE d.rn > 1
)
UPDATE gitea_auto_transitions gt
SET target_status_id = dups.survivor_id
FROM dups
WHERE gt.target_status_id = dups.dup_id;

-- project_status_visibility has a composite primary key, so drop the rows
-- that would collide before repointing the remaining ones.
WITH ranked AS (
    SELECT id, workspace_id, slug,
           ROW_NUMBER() OVER (PARTITION BY workspace_id, slug ORDER BY position, id) AS rn
    FROM team_statuses
    WHERE workspace_id IS NOT NULL
),
dups AS (
    SELECT d.id AS dup_id, k.id AS survivor_id
    FROM ranked d
    INNER JOIN ranked k
        ON k.workspace_id = d.workspace_id AND k.slug = d.slug AND k.rn = 1
    WHERE d.rn > 1
)
DELETE FROM project_status_visibility psv
USING dups
WHERE psv.status_id = dups.dup_id
  AND EXISTS (
      SELECT 1 FROM project_status_visibility existing
      WHERE existing.project_id = psv.project_id
        AND existing.status_id = dups.survivor_id
  );

WITH ranked AS (
    SELECT id, workspace_id, slug,
           ROW_NUMBER() OVER (PARTITION BY workspace_id, slug ORDER BY position, id) AS rn
    FROM team_statuses
    WHERE workspace_id IS NOT NULL
),
dups AS (
    SELECT d.id AS dup_id, k.id AS survivor_id
    FROM ranked d
    INNER JOIN ranked k
        ON k.workspace_id = d.workspace_id AND k.slug = d.slug AND k.rn = 1
    WHERE d.rn > 1
)
UPDATE project_status_visibility psv
SET status_id = dups.survivor_id
FROM dups
WHERE psv.status_id = dups.dup_id;

-- Remove the duplicates now that nothing references them
WITH ranked AS (
    SELECT id, workspace_id, slug,
           ROW_NUMBER() OVER (PARTITION BY workspace_id, slug ORDER BY position, id) AS rn
    FROM team_statuses
    WHERE workspace_id IS NOT NULL
)
DELETE FROM team_statuses ts
USING ranked
WHERE ts.id = ranked.id AND ranked.rn > 1;

-- 5. Seed the default set for every workspace that has no statuses yet
INSERT INTO team_statuses (workspace_id, team_id, name, slug, category, position, is_default)
SELECT w.id, NULL, s.name, s.slug, s.category, s.position, s.is_default
FROM workspaces w
CROSS JOIN (VALUES
    ('Backlog',     'backlog',     'backlog',   0, true),
    ('Todo',        'todo',        'unstarted', 1, false),
    ('In Progress', 'in_progress', 'started',   2, false),
    ('In Review',   'in_review',   'started',   3, false),
    ('Done',        'done',        'completed', 4, false),
    ('Cancelled',   'cancelled',   'cancelled', 5, false)
) AS s(name, slug, category, position, is_default)
WHERE NOT EXISTS (
    SELECT 1 FROM team_statuses ts WHERE ts.workspace_id = w.id
);

-- 6. One status per (workspace, slug)
CREATE UNIQUE INDEX IF NOT EXISTS idx_team_statuses_workspace_slug
    ON team_statuses(workspace_id, slug) WHERE workspace_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_team_statuses_workspace ON team_statuses(workspace_id, position);
