-- The puzzle of the day is practice: it does not move the puzzle rating. What
-- is kept is how each person's first try of a day's puzzle went (the page shows
-- it, and a streak of days can be counted from it later).
CREATE TABLE daily_puzzle_results (
    user_id    CHAR(16)    NOT NULL,
    day        DATE        NOT NULL,                   -- UTC
    puzzle_id  VARCHAR(12) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    solved     TINYINT(1)  NOT NULL,
    created_at DATETIME(3) NOT NULL,
    PRIMARY KEY (user_id, day),
    CONSTRAINT fk_daily_puzzle_results_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
