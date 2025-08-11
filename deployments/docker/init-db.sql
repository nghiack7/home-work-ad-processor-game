-- Create database schema for ad processing system
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Create ads table
CREATE TABLE IF NOT EXISTS ads (
    ad_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    title VARCHAR(500) NOT NULL,
    game_family VARCHAR(100) NOT NULL,
    target_audience JSONB NOT NULL,
    priority INTEGER NOT NULL CHECK (priority BETWEEN 1 AND 5),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    max_wait_time INTEGER NOT NULL DEFAULT 300,
    status VARCHAR(20) NOT NULL DEFAULT 'queued',
    processing_started_at TIMESTAMP WITH TIME ZONE,
    processed_at TIMESTAMP WITH TIME ZONE,
    version INTEGER NOT NULL DEFAULT 1,
    shard_key VARCHAR(10) GENERATED ALWAYS AS (substring(ad_id::text, 1, 2)) STORED
);

-- Create indexes for performance
CREATE INDEX IF NOT EXISTS idx_ads_status_priority ON ads(status, priority, created_at);
CREATE INDEX IF NOT EXISTS idx_ads_game_family ON ads(game_family);
CREATE INDEX IF NOT EXISTS idx_ads_created_at ON ads(created_at);
CREATE INDEX IF NOT EXISTS idx_ads_status ON ads(status);
CREATE INDEX IF NOT EXISTS idx_ads_priority ON ads(priority);
CREATE INDEX IF NOT EXISTS idx_ads_shard_key ON ads(shard_key);

-- Create commands table for AI command tracking
CREATE TABLE IF NOT EXISTS commands (
    command_id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    original_text TEXT NOT NULL,
    command_type VARCHAR(50) NOT NULL,
    intent VARCHAR(100) NOT NULL,
    parameters JSONB,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    result JSONB,
    error_message TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    executed_at TIMESTAMP WITH TIME ZONE,
    execution_time_ms INTEGER
);

-- Create index for commands
CREATE INDEX IF NOT EXISTS idx_commands_status ON commands(status);
CREATE INDEX IF NOT EXISTS idx_commands_created_at ON commands(created_at);
CREATE INDEX IF NOT EXISTS idx_commands_type ON commands(command_type);

-- Insert sample data for testing
INSERT INTO ads (title, game_family, target_audience, priority, max_wait_time) VALUES
('Dragon Quest Adventures', 'RPG-Fantasy', '["18-34", "RPG Gamers"]', 3, 300),
('Space Shooter Elite', 'Action', '["16-25", "Action Gamers"]', 4, 240),
('Chess Master Pro', 'Strategy', '["25-45", "Strategy Gamers"]', 2, 600),
('Racing Thunder', 'Racing', '["18-30", "Racing Fans"]', 5, 180),
('Puzzle Kingdom', 'Puzzle', '["30-50", "Casual Gamers"]', 1, 900);

-- Create user for application
CREATE USER IF NOT EXISTS adprocessing WITH PASSWORD 'adprocessing123';
GRANT ALL PRIVILEGES ON DATABASE adprocessing TO adprocessing;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO adprocessing;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO adprocessing;