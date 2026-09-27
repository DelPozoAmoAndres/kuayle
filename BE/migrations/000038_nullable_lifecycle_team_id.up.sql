-- Allow team_id to be NULL in issue_lifecycle_events for issues without a team
ALTER TABLE issue_lifecycle_events ALTER COLUMN team_id DROP NOT NULL;

-- Update the trigger function to handle NULL team_id
CREATE OR REPLACE FUNCTION issue_lifecycle_log_event()
RETURNS TRIGGER AS $$
DECLARE
    old_cat VARCHAR(50);
    new_cat VARCHAR(50);
BEGIN
    IF NEW.status_id IS NOT NULL THEN
        SELECT category INTO new_cat FROM team_statuses WHERE id = NEW.status_id;
    END IF;
    IF new_cat IS NULL THEN
        new_cat := CASE NEW.status::text
            WHEN 'done'        THEN 'completed'
            WHEN 'completed'   THEN 'completed'
            WHEN 'cancelled'   THEN 'cancelled'
            WHEN 'started'     THEN 'started'
            WHEN 'in_progress' THEN 'started'
            WHEN 'in_review'   THEN 'started'
            WHEN 'unstarted'   THEN 'unstarted'
            WHEN 'todo'        THEN 'unstarted'
            ELSE 'backlog'
        END;
    END IF;

    IF TG_OP = 'INSERT' THEN
        INSERT INTO issue_lifecycle_events
            (issue_id, from_status_id, to_status_id, event_type, from_category, to_category, created_at, workspace_id, team_id, project_id, cycle_id)
        VALUES (NEW.id, NULL, NEW.status_id, 'created', NULL, new_cat, NEW.created_at, NEW.workspace_id, NEW.team_id, NEW.project_id, NEW.cycle_id);
        RETURN NEW;
    END IF;

    IF OLD.status_id IS NOT NULL THEN
        SELECT category INTO old_cat FROM team_statuses WHERE id = OLD.status_id;
    END IF;
    IF old_cat IS NULL THEN
        old_cat := CASE OLD.status::text
            WHEN 'done'        THEN 'completed'
            WHEN 'completed'   THEN 'completed'
            WHEN 'cancelled'   THEN 'cancelled'
            WHEN 'started'     THEN 'started'
            WHEN 'in_progress' THEN 'started'
            WHEN 'in_review'   THEN 'started'
            WHEN 'unstarted'   THEN 'unstarted'
            WHEN 'todo'        THEN 'unstarted'
            ELSE 'backlog'
        END;
    END IF;

    IF OLD.status_id IS DISTINCT FROM NEW.status_id OR OLD.status IS DISTINCT FROM NEW.status THEN
        INSERT INTO issue_lifecycle_events
            (issue_id, from_status_id, to_status_id, event_type, from_category, to_category, created_at, workspace_id, team_id, project_id, cycle_id)
        VALUES (NEW.id, OLD.status_id, NEW.status_id, 'status_changed', old_cat, new_cat, NOW(), NEW.workspace_id, NEW.team_id, NEW.project_id, NEW.cycle_id);
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
