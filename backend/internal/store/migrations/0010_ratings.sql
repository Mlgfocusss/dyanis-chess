-- Glicko-2 ratings of people, one per kind of game (see internal/ratings): bullet, blitz, rapid and
-- the puzzle duel. A person with no row there has not played the kind yet.
CREATE TABLE ratings (
    user_id    CHAR(16)    NOT NULL,
    category   VARCHAR(12) CHARACTER SET ascii NOT NULL,
    rating     DOUBLE      NOT NULL,
    rd         DOUBLE      NOT NULL,                 -- rating deviation
    volatility DOUBLE      NOT NULL,
    games      INT         NOT NULL DEFAULT 0,
    wins       INT         NOT NULL DEFAULT 0,
    draws      INT         NOT NULL DEFAULT 0,
    losses     INT         NOT NULL DEFAULT 0,
    updated_at DATETIME(3) NOT NULL,
    PRIMARY KEY (user_id, category),
    CONSTRAINT fk_ratings_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- Finished puzzle duels (see internal/puzzles/duel.go); the duel itself lives in memory.
CREATE TABLE duels (
    id          CHAR(16)    NOT NULL,
    a_id        CHAR(16)    NULL,
    b_id        CHAR(16)    NULL,
    a_score     INT         NOT NULL,
    b_score     INT         NOT NULL,
    a_mistakes  INT         NOT NULL,
    b_mistakes  INT         NOT NULL,
    result      VARCHAR(8)  NOT NULL,                -- "a", "b" or "draw"
    created_at  DATETIME(3) NOT NULL,
    PRIMARY KEY (id),
    KEY idx_duels_a (a_id, created_at),
    KEY idx_duels_b (b_id, created_at),
    CONSTRAINT fk_duels_a FOREIGN KEY (a_id) REFERENCES users (id) ON DELETE SET NULL,
    CONSTRAINT fk_duels_b FOREIGN KEY (b_id) REFERENCES users (id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
