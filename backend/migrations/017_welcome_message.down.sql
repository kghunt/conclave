ALTER TABLE servers
  DROP COLUMN IF EXISTS welcome_channel_id,
  DROP COLUMN IF EXISTS welcome_message;
