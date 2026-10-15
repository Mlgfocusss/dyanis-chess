-- Games between two people. The whole game (moves, clocks, offers) is one
-- JSON document that the server rewrites after every change, so a restart
-- loses nothing; the columns beside it are what lists and lookups need.
CREATE TABLE games (
    id         CHAR(16)    NOT NULL,
    white_id   CHAR(16)    NULL,
    black_id   CHAR(16)    NULL,
    status     VARCHAR(10) NOT NULL,            -- waiting, playing, ended, aborted
    result     VARCHAR(8)  NULL,                -- 1-0, 0-1, 1/2-1/2
    state      JSON        NOT NULL,            -- play.saved
    created_at DATETIME(3) NOT NULL,
    updated_at DATETIME(3) NOT NULL,
    PRIMARY KEY (id),
    KEY idx_games_white (white_id, updated_at),
    KEY idx_games_black (black_id, updated_at),
    CONSTRAINT fk_games_white FOREIGN KEY (white_id) REFERENCES users (id) ON DELETE SET NULL,
    CONSTRAINT fk_games_black FOREIGN KEY (black_id) REFERENCES users (id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
