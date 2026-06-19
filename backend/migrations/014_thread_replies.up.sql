ALTER TABLE thread_messages ADD COLUMN reply_to_id UUID REFERENCES thread_messages(id) ON DELETE SET NULL;
