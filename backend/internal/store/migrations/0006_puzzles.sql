-- Puzzles: a subset of the Lichess puzzle database (CC0), imported by
-- backend/cmd/puzzleimport, and each person's puzzle rating and attempts.
-- Puzzle ids are case-sensitive ("00sHx"), hence the binary collation.
CREATE TABLE puzzles (
    id         VARCHAR(12)  CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    fen        VARCHAR(100) NOT NULL,                  -- before the opponent's first move
    moves      VARCHAR(255) NOT NULL,                  -- UCI, space separated; the first is the opponent's
    rating     SMALLINT     NOT NULL,
    popularity SMALLINT     NOT NULL,
    plays      INT          NOT NULL,
    themes     VARCHAR(255) NOT NULL,                  -- space separated
    PRIMARY KEY (id),
    KEY idx_puzzles_rating (rating)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- One row per puzzle and theme; the rating is copied in so that "a puzzle of
-- this theme near my rating" is a single range scan.
CREATE TABLE puzzle_themes (
    theme     VARCHAR(40)  CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    rating    SMALLINT     NOT NULL,
    puzzle_id VARCHAR(12)  CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    PRIMARY KEY (theme, rating, puzzle_id),
    KEY idx_puzzle_themes_puzzle (puzzle_id),
    CONSTRAINT fk_puzzle_themes_puzzle FOREIGN KEY (puzzle_id) REFERENCES puzzles (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE puzzle_ratings (
    user_id    CHAR(16)    NOT NULL,
    rating     INT         NOT NULL,
    attempts   INT         NOT NULL DEFAULT 0,
    solved     INT         NOT NULL DEFAULT 0,
    updated_at DATETIME(3) NOT NULL,
    PRIMARY KEY (user_id),
    CONSTRAINT fk_puzzle_ratings_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- The first try at a puzzle counts for the rating; later tries are practice.
-- No foreign key to puzzles: the subset may be re-imported.
CREATE TABLE puzzle_attempts (
    id            BIGINT      NOT NULL AUTO_INCREMENT,
    user_id       CHAR(16)    NOT NULL,
    puzzle_id     VARCHAR(12) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    solved        TINYINT(1)  NOT NULL,
    rating_before INT         NOT NULL,
    rating_after  INT         NOT NULL,
    created_at    DATETIME(3) NOT NULL,
    PRIMARY KEY (id),
    KEY idx_puzzle_attempts_user_puzzle (user_id, puzzle_id),
    KEY idx_puzzle_attempts_user_time (user_id, created_at),
    CONSTRAINT fk_puzzle_attempts_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
