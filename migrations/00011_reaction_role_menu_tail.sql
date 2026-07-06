-- +goose Up

-- Reaction-role menus gain a canvas-owned follow-up flow: the editable steps
-- the admin wired after the "apply picked roles" spine on the menu's built-in
-- automation ("reactionroles.menu.<id>"). It runs as a durable automation run
-- after a member picks roles from the posted menu. Saved via the dedicated
-- /reaction-roles/:mid/actions endpoint; the dashboard's menu upsert never
-- writes it, so a settings save can't clobber the flow.
ALTER TABLE reaction_role_menus
    ADD COLUMN tail JSONB NOT NULL DEFAULT '[]'::jsonb;

-- +goose Down
ALTER TABLE reaction_role_menus DROP COLUMN IF EXISTS tail;
