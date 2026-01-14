-- Migration script to make input_bucket and input_key optional
-- This allows jobs to run without input files (e.g., scheduled tasks, pure computation)
-- 
-- Usage:
--   mysql -u root -p cospace < migrate_input_optional.sql
--   Or copy and paste into MySQL client

USE cospace;

-- Modify input_bucket to allow NULL
ALTER TABLE jobs 
MODIFY COLUMN input_bucket VARCHAR(255) NULL 
COMMENT 'OSS input bucket name (optional - jobs can run without input files)';

-- Modify input_key to allow NULL
ALTER TABLE jobs 
MODIFY COLUMN input_key VARCHAR(512) NULL 
COMMENT 'OSS input object key (optional - jobs can run without input files)';

-- Verify the changes
-- You can run: DESCRIBE jobs; to see the updated column definitions
