ALTER TABLE servers
  ADD COLUMN welcome_channel_id UUID REFERENCES channels(id) ON DELETE SET NULL,
  ADD COLUMN welcome_message TEXT NOT NULL DEFAULT '';
