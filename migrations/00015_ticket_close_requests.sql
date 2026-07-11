-- +goose Up
-- Close requests: staff ask the opener to confirm a close (optionally
-- auto-accepting after a deadline). close_requested_by <> 0 marks a pending
-- request; close_request_at is the auto-accept deadline (NULL = wait forever).
ALTER TABLE tickets
    ADD COLUMN close_requested_by BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN close_request_reason TEXT NOT NULL DEFAULT '',
    ADD COLUMN close_request_at TIMESTAMPTZ;

-- The sweep scans open tickets whose auto-accept deadline has passed.
CREATE INDEX idx_tickets_close_request_due ON tickets (close_request_at)
    WHERE status = 'open' AND close_request_at IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_tickets_close_request_due;
ALTER TABLE tickets
    DROP COLUMN IF EXISTS close_requested_by,
    DROP COLUMN IF EXISTS close_request_reason,
    DROP COLUMN IF EXISTS close_request_at;
