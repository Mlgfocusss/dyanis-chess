-- How each person has fared against each bot. Points are earned once per bot,
-- by the first win (the bots page shows the bot as beaten from then on).
CREATE TABLE bot_progress (
    user_id      CHAR(16)    NOT NULL,
    bot_id       VARCHAR(32) NOT NULL,
    wins         INT         NOT NULL DEFAULT 0,
    draws        INT         NOT NULL DEFAULT 0,
    losses       INT         NOT NULL DEFAULT 0,
    points       INT         NOT NULL DEFAULT 0,       -- earned from this bot so far
    first_win_at DATETIME(3) NULL,
    updated_at   DATETIME(3) NOT NULL,
    PRIMARY KEY (user_id, bot_id),
    CONSTRAINT fk_bot_progress_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
