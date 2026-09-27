-- Allow status_id to be NULL for issues without a team
ALTER TABLE issues ALTER COLUMN status_id DROP NOT NULL;
