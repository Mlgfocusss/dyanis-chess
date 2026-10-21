-- Puzzle points: the mode where a first try that is solved earns points and the puzzle rating stays
-- (settings → Игровой процесс → Задачи). The server counts them; one row per user for the total and
-- the series of solved puzzles in a row, one row per user and day for "today".
CREATE TABLE puzzle_points (
    user_id     CHAR(16)    NOT NULL,
    points      INT         NOT NULL DEFAULT 0,
    streak      INT         NOT NULL DEFAULT 0,
    best_streak INT         NOT NULL DEFAULT 0,
    updated_at  DATETIME(3) NOT NULL,
    PRIMARY KEY (user_id),
    CONSTRAINT fk_puzzle_points_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE puzzle_points_days (
    user_id CHAR(16) NOT NULL,
    day     DATE     NOT NULL,                  -- UTC
    points  INT      NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, day),
    CONSTRAINT fk_puzzle_points_days_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
