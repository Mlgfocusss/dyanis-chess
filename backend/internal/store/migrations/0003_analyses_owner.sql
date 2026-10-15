-- A review belongs to whoever started it while signed in (NULL = anonymous;
-- the link still opens for anyone). summary is a few figures for the
-- "my games" list, so listing never has to read whole reports.
ALTER TABLE analyses
    ADD COLUMN user_id CHAR(16) NULL,
    ADD COLUMN summary JSON NULL,
    ADD KEY idx_analyses_user (user_id, created_at),
    ADD CONSTRAINT fk_analyses_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE SET NULL;
