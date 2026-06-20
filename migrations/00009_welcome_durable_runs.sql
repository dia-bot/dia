-- +goose Up

-- Feature-owned flows (Welcome's join/leave + button-click programs) now run on
-- the same durable machinery as user automations: they persist to
-- automation_runs and resume through the automations plugin's auto: component /
-- modal handlers + wait scheduler. Those runs have no row in `automations`
-- (Welcome is config-owned, not a stored automation), so automation_id becomes a
-- free-form label (a real automation UUID, or a key like 'welcome.join') rather
-- than a foreign key. Drop the FK; keep the column (still indexed for the Runs
-- filter).
ALTER TABLE automation_runs DROP CONSTRAINT IF EXISTS automation_runs_automation_id_fkey;

-- +goose Down

-- Re-adding the FK requires every run to point at a live automation; drop the
-- feature-owned runs (which never had one) first so the constraint can apply.
DELETE FROM automation_runs r
    WHERE NOT EXISTS (SELECT 1 FROM automations a WHERE a.id = r.automation_id);
ALTER TABLE automation_runs
    ADD CONSTRAINT automation_runs_automation_id_fkey
    FOREIGN KEY (automation_id) REFERENCES automations(id) ON DELETE CASCADE;
