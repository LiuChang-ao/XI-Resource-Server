-- Migration script to extend command field length from VARCHAR(1024) to VARCHAR(8192)
-- This allows longer command lines with complex arguments and paths
-- 
-- Usage:
--   mysql -u root -p <database_name> < migrate_command_length.sql
--   Or copy and paste into MySQL client
--
-- Note: Replace <database_name> with your actual database name (e.g., xiresource, cospace)

-- Modify command column to allow longer commands
ALTER TABLE jobs 
MODIFY COLUMN command VARCHAR(8192) 
COMMENT 'Command to execute on agent';

-- Verify the changes
-- You can run: DESCRIBE jobs; to see the updated column definition
