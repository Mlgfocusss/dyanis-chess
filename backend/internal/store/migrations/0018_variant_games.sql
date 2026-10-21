-- Games of the chess variants ("Другие шахматы", internal/vplay), between two people, unrated. Like
-- `games`: the whole game (options, actions, clocks, offers) is one JSON document that the server rewrites
-- after every change, so a restart loses nothing; the columns beside it are what lists need.
CREATE TABLE variant_games (
    id         CHAR(16)    NOT NULL,
    variant    VARCHAR(32) CHARACTER SET ascii NOT NULL,   -- the id of the variant, e.g. "atomic"
    white_id   CHAR(16)    NULL,
    black_id   CHAR(16)    NULL,
    status     VARCHAR(10) NOT NULL,                       -- waiting, playing, ended, aborted
    result     VARCHAR(8)  NULL,                           -- 1-0, 0-1, 1/2-1/2
    state      JSON        NOT NULL,                       -- vplay.saved
    created_at DATETIME(3) NOT NULL,
    updated_at DATETIME(3) NOT NULL,
    PRIMARY KEY (id),
    KEY idx_variant_games_white (white_id, updated_at),
    KEY idx_variant_games_black (black_id, updated_at),
    CONSTRAINT fk_variant_games_white FOREIGN KEY (white_id) REFERENCES users (id) ON DELETE SET NULL,
    CONSTRAINT fk_variant_games_black FOREIGN KEY (black_id) REFERENCES users (id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
