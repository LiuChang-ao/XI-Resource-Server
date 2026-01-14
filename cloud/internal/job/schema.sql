-- MySQL Database Schema for XI Resource Server
-- 
-- This script creates the jobs table and required indexes.
-- 
-- Usage:
--   1. Create database (if not exists):
--      CREATE DATABASE IF NOT EXISTS xiresource CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
--      USE xiresource;
--   
--   2. Run this script:
--      mysql -u root -p xiresource < schema.sql
--      Or copy and paste into MySQL client

-- Create jobs table
CREATE TABLE IF NOT EXISTS jobs (
    job_id VARCHAR(255) PRIMARY KEY COMMENT 'Job UUID',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT 'Job creation timestamp',
    status VARCHAR(50) NOT NULL COMMENT 'Job status: PENDING, ASSIGNED, RUNNING, SUCCEEDED, FAILED, CANCELED, LOST',
    input_bucket VARCHAR(255) COMMENT 'OSS input bucket name (optional - jobs can run without input files)',
    input_key VARCHAR(512) COMMENT 'OSS input object key (optional - jobs can run without input files)',
    output_bucket VARCHAR(255) NOT NULL COMMENT 'OSS output bucket name',
    output_key VARCHAR(512) COMMENT 'OSS output object key (optional)',
    output_prefix VARCHAR(512) COMMENT 'OSS output prefix (optional, format: jobs/{job_id}/{attempt_id}/)',
    attempt_id INT NOT NULL DEFAULT 1 COMMENT 'Job attempt number (starts at 1)',
    assigned_agent_id VARCHAR(255) COMMENT 'ID of agent assigned to this job',
    lease_id VARCHAR(255) COMMENT 'Lease ID for job execution',
    lease_deadline DATETIME COMMENT 'Lease expiration time',
    command VARCHAR(1024) COMMENT 'Command to execute on agent',
    CONSTRAINT chk_attempt_id CHECK (attempt_id >= 1),
    CONSTRAINT chk_status CHECK (status IN ('PENDING', 'ASSIGNED', 'RUNNING', 'SUCCEEDED', 'FAILED', 'CANCELED', 'LOST'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Job management table';

-- Create indexes for better query performance
-- Note: IF NOT EXISTS for CREATE INDEX is only supported in MySQL 8.0.13+
-- For older MySQL versions, manually check if index exists or ignore duplicate index errors

-- For MySQL 8.0.13+, use: CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
-- For MySQL < 8.0.13, use the following approach:

-- Create index on status (ignore error if already exists)
CREATE INDEX idx_jobs_status ON jobs(status);

-- Create index on created_at (ignore error if already exists)
CREATE INDEX idx_jobs_created_at ON jobs(created_at);

-- Create index on assigned_agent_id (ignore error if already exists)
CREATE INDEX idx_jobs_assigned_agent ON jobs(assigned_agent_id);
