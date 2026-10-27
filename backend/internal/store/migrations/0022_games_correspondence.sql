-- Correspondence games (days per move): who has to move and until when, to count "your turn" and to
-- load the unfinished ones when the server starts without reading every game.
ALTER TABLE games
    ADD COLUMN corr    TINYINT(1)  NOT NULL DEFAULT 0,
    ADD COLUMN turn_id CHAR(16)    NULL,
    ADD COLUMN due_at  DATETIME(3) NULL,
    ADD KEY idx_games_corr (corr, status),
    ADD KEY idx_games_turn (turn_id, corr, status);
