-- +goose Up
-- +goose StatementBegin

-- Add version column for optimistic locking support
ALTER TABLE ads ADD COLUMN version INTEGER NOT NULL DEFAULT 1;

-- Create index on version for performance
CREATE INDEX idx_ads_version ON ads (version);

-- Update comment to document the version column
COMMENT ON COLUMN ads.version IS 'Version number for optimistic locking concurrency control';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Drop the version column and its index
DROP INDEX IF EXISTS idx_ads_version;
ALTER TABLE ads DROP COLUMN IF EXISTS version;

-- +goose StatementEnd
