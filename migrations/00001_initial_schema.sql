-- +goose Up
-- +goose StatementBegin
-- Initial Schema Migration
-- Version: 001
-- Description: Create initial tables for ad processing system
-- Created: 2025-08-08

-- Create database user if not exists
-- This should be run by a superuser
-- CREATE USER ad_processor WITH PASSWORD 'change_me_in_production';

-- Create database if not exists
-- CREATE DATABASE ad_processing_prod OWNER ad_processor;

-- Use the database
-- \c ad_processing_prod;

-- Enable necessary extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";
CREATE EXTENSION IF NOT EXISTS "btree_gin";

-- Create custom types
CREATE TYPE ad_status AS ENUM ('queued', 'processing', 'completed', 'failed');
CREATE TYPE command_status AS ENUM ('pending', 'executing', 'completed', 'failed');
CREATE TYPE command_type AS ENUM ('queue_modification', 'system_configuration', 'status_query', 'analytics');

-- Ads table - main table for ad metadata
CREATE TABLE ads (
    ad_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    title VARCHAR(500) NOT NULL,
    game_family VARCHAR(100) NOT NULL,
    target_audience JSONB NOT NULL DEFAULT '[]'::jsonb,
    priority INTEGER NOT NULL CHECK (priority BETWEEN 1 AND 5) DEFAULT 3,
    max_wait_time INTEGER NOT NULL DEFAULT 300, -- seconds
    status ad_status NOT NULL DEFAULT 'queued',
    
    -- Timestamps
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    queued_at TIMESTAMP WITH TIME ZONE,
    processing_started_at TIMESTAMP WITH TIME ZONE,
    processed_at TIMESTAMP WITH TIME ZONE,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    
    -- Additional metadata
    processing_duration_ms INTEGER, -- How long processing took
    worker_id VARCHAR(100), -- Which worker processed this ad
    error_message TEXT, -- Error details if failed
    retry_count INTEGER NOT NULL DEFAULT 0,
    
    -- Sharding key for horizontal scaling (simplified for compatibility)
    shard_key VARCHAR(10) DEFAULT substring(md5(random()::text), 1, 2),
    
    -- Audit fields
    created_by VARCHAR(100) DEFAULT 'system',
    updated_by VARCHAR(100) DEFAULT 'system'
);

-- Commands table - for AI agent command tracking
CREATE TABLE commands (
    command_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    original_text TEXT NOT NULL,
    parsed_intent VARCHAR(100) NOT NULL,
    command_type command_type NOT NULL,
    parameters JSONB NOT NULL DEFAULT '{}'::jsonb,
    status command_status NOT NULL DEFAULT 'pending',
    
    -- Execution details
    execution_started_at TIMESTAMP WITH TIME ZONE,
    execution_completed_at TIMESTAMP WITH TIME ZONE,
    execution_duration_ms INTEGER,
    result JSONB,
    error_message TEXT,
    
    -- Metadata
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_by VARCHAR(100) DEFAULT 'ai_agent',
    ip_address INET,
    user_agent TEXT
);

-- Queue statistics table - for analytics and monitoring
CREATE TABLE queue_statistics (
    id BIGSERIAL PRIMARY KEY,
    timestamp TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    
    -- Queue metrics
    total_ads INTEGER NOT NULL DEFAULT 0,
    priority_1_count INTEGER NOT NULL DEFAULT 0,
    priority_2_count INTEGER NOT NULL DEFAULT 0,
    priority_3_count INTEGER NOT NULL DEFAULT 0,
    priority_4_count INTEGER NOT NULL DEFAULT 0,
    priority_5_count INTEGER NOT NULL DEFAULT 0,
    
    -- Processing metrics
    processing_rate_per_minute DECIMAL(10,2) DEFAULT 0,
    avg_processing_time_ms INTEGER DEFAULT 0,
    avg_wait_time_ms INTEGER DEFAULT 0,
    
    -- System metrics
    active_workers INTEGER DEFAULT 0,
    failed_processing_count INTEGER DEFAULT 0,
    
    -- Partition by date for better performance (simplified)
    recorded_date DATE DEFAULT CURRENT_DATE
);

-- System configuration table
CREATE TABLE system_config (
    key VARCHAR(100) PRIMARY KEY,
    value JSONB NOT NULL,
    description TEXT,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_by VARCHAR(100) DEFAULT 'system'
);

-- Create indexes for performance (non-concurrent for initial migration)

-- Ads table indexes
CREATE INDEX idx_ads_status_priority_created 
    ON ads (status, priority DESC, created_at ASC);

CREATE INDEX idx_ads_game_family 
    ON ads (game_family);

CREATE INDEX idx_ads_created_at 
    ON ads (created_at);

CREATE INDEX idx_ads_status_processing_started 
    ON ads (status, processing_started_at) 
    WHERE status = 'processing';

CREATE INDEX idx_ads_shard_key 
    ON ads (shard_key);

-- Only create GIN indexes if extensions are available
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'btree_gin') THEN
        CREATE INDEX idx_ads_target_audience_gin ON ads USING GIN (target_audience);
    END IF;
    
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_trgm') THEN
        CREATE INDEX idx_ads_title_trgm ON ads USING GIN (title gin_trgm_ops);
    END IF;
EXCEPTION WHEN OTHERS THEN
    -- Ignore errors if extensions are not available
    RAISE NOTICE 'Some indexes could not be created - extensions may not be available';
END $$;

-- Commands table indexes
CREATE INDEX idx_commands_created_at 
    ON commands (created_at DESC);

CREATE INDEX idx_commands_status 
    ON commands (status);

CREATE INDEX idx_commands_type_intent 
    ON commands (command_type, parsed_intent);

-- Queue statistics indexes
CREATE INDEX idx_queue_stats_timestamp 
    ON queue_statistics (timestamp DESC);

CREATE INDEX idx_queue_stats_date 
    ON queue_statistics (recorded_date);

-- Create partitions for queue_statistics table (monthly partitions)
-- This is for future scaling - we'll create partitions as needed

-- Functions and triggers

-- Update timestamp function
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Create triggers
CREATE TRIGGER update_ads_updated_at 
    BEFORE UPDATE ON ads 
    FOR EACH ROW 
    EXECUTE FUNCTION update_updated_at_column();

-- Function to calculate wait time
CREATE OR REPLACE FUNCTION calculate_wait_time(
    created_at TIMESTAMP WITH TIME ZONE,
    processing_started_at TIMESTAMP WITH TIME ZONE DEFAULT NULL
) RETURNS INTERVAL AS $$
BEGIN
    IF processing_started_at IS NULL THEN
        RETURN NOW() - created_at;
    ELSE
        RETURN processing_started_at - created_at;
    END IF;
END;
$$ LANGUAGE plpgsql IMMUTABLE;

-- Function to get queue position (approximate)
CREATE OR REPLACE FUNCTION get_approximate_queue_position(
    input_ad_id UUID
) RETURNS INTEGER AS $$
DECLARE
    ad_priority INTEGER;
    ad_created_at TIMESTAMP WITH TIME ZONE;
    position INTEGER;
BEGIN
    -- Get the ad's priority and creation time
    SELECT priority, created_at 
    INTO ad_priority, ad_created_at
    FROM ads 
    WHERE ad_id = input_ad_id AND status = 'queued';
    
    IF NOT FOUND THEN
        RETURN NULL;
    END IF;
    
    -- Count ads with higher priority or same priority but created earlier
    SELECT COUNT(*) + 1
    INTO position
    FROM ads
    WHERE status = 'queued'
    AND (
        priority > ad_priority
        OR (priority = ad_priority AND created_at < ad_created_at)
    );
    
    RETURN position;
END;
$$ LANGUAGE plpgsql;

-- Insert initial system configuration
INSERT INTO system_config (key, value, description) VALUES
    ('anti_starvation_enabled', 'true', 'Enable anti-starvation mechanism'),
    ('max_wait_time_seconds', '600', 'Maximum wait time before priority boost'),
    ('worker_count', '10', 'Number of processing workers'),
    ('batch_size', '20', 'Batch size for processing'),
    ('queue_shard_count', '4', 'Number of queue shards'),
    ('priority_boost_interval_seconds', '60', 'How often to check for starvation')
ON CONFLICT (key) DO NOTHING;

-- Grant permissions
-- GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO ad_processor;
-- GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO ad_processor;
-- GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO ad_processor;

-- Create a view for queue analytics
CREATE VIEW queue_analytics AS
SELECT 
    COUNT(*) as total_queued,
    COUNT(*) FILTER (WHERE priority = 1) as priority_1,
    COUNT(*) FILTER (WHERE priority = 2) as priority_2,
    COUNT(*) FILTER (WHERE priority = 3) as priority_3,
    COUNT(*) FILTER (WHERE priority = 4) as priority_4,
    COUNT(*) FILTER (WHERE priority = 5) as priority_5,
    AVG(EXTRACT(EPOCH FROM calculate_wait_time(created_at))) as avg_wait_seconds,
    MAX(EXTRACT(EPOCH FROM calculate_wait_time(created_at))) as max_wait_seconds,
    COUNT(*) FILTER (WHERE created_at > NOW() - INTERVAL '1 hour') as queued_last_hour,
    COUNT(*) FILTER (WHERE created_at > NOW() - INTERVAL '1 day') as queued_last_day
FROM ads 
WHERE status = 'queued';

-- Create a view for processing analytics
CREATE VIEW processing_analytics AS
SELECT 
    COUNT(*) FILTER (WHERE status = 'completed') as completed_total,
    COUNT(*) FILTER (WHERE status = 'failed') as failed_total,
    COUNT(*) FILTER (WHERE status = 'processing') as currently_processing,
    AVG(processing_duration_ms) as avg_processing_ms,
    COUNT(*) FILTER (WHERE processed_at > NOW() - INTERVAL '1 hour') as processed_last_hour,
    COUNT(*) FILTER (WHERE processed_at > NOW() - INTERVAL '1 day') as processed_last_day,
    AVG(processing_duration_ms) FILTER (WHERE processed_at > NOW() - INTERVAL '1 hour') as avg_processing_ms_last_hour
FROM ads 
WHERE status IN ('completed', 'failed', 'processing');

-- Add some helpful comments
COMMENT ON TABLE ads IS 'Main table storing ad metadata and processing status';
COMMENT ON TABLE commands IS 'AI agent commands and their execution results';
COMMENT ON TABLE queue_statistics IS 'Historical queue metrics for analytics';
COMMENT ON TABLE system_config IS 'System-wide configuration parameters';

COMMENT ON COLUMN ads.shard_key IS 'Auto-generated sharding key based on ad_id hash';
COMMENT ON COLUMN ads.target_audience IS 'JSONB array of target audience segments';
COMMENT ON COLUMN ads.processing_duration_ms IS 'Time taken to process this ad in milliseconds';

-- Migration metadata
INSERT INTO system_config (key, value, description) VALUES
    ('migration_version', '"001"', 'Current database migration version'),
    ('migration_applied_at', to_jsonb(NOW()::text), 'When the last migration was applied')
ON CONFLICT (key) DO UPDATE SET 
    value = EXCLUDED.value,
    updated_at = NOW();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Drop views
DROP VIEW IF EXISTS processing_analytics;
DROP VIEW IF EXISTS queue_analytics;

-- Drop triggers
DROP TRIGGER IF EXISTS update_ads_updated_at ON ads;

-- Drop functions
DROP FUNCTION IF EXISTS update_updated_at_column();
DROP FUNCTION IF EXISTS calculate_wait_time(TIMESTAMP WITH TIME ZONE, TIMESTAMP WITH TIME ZONE);
DROP FUNCTION IF EXISTS get_approximate_queue_position(UUID);

-- Drop tables (in reverse order of creation)
DROP TABLE IF EXISTS system_config;
DROP TABLE IF EXISTS queue_statistics;
DROP TABLE IF EXISTS commands;
DROP TABLE IF EXISTS ads;

-- Drop custom types
DROP TYPE IF EXISTS command_type;
DROP TYPE IF EXISTS command_status;
DROP TYPE IF EXISTS ad_status;

-- Drop extensions (be careful with this in production)
-- DROP EXTENSION IF EXISTS "btree_gin";
-- DROP EXTENSION IF EXISTS "pg_trgm";
-- DROP EXTENSION IF EXISTS "uuid-ossp";

-- +goose StatementEnd