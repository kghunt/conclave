ALTER TABLE dm_conversations
  DROP COLUMN IF EXISTS user1_read_at,
  DROP COLUMN IF EXISTS user2_read_at;
