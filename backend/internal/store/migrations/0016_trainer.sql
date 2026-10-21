-- Puzzles done as practice in "Задачи → По темам": they do not touch the rating and are not kept
-- anywhere else, but the trainer's plan for the day ("5 задач по слабой теме") has to count them.
-- One row per person, day (UTC) and puzzle, so repeating a puzzle does not count twice.
CREATE TABLE trainer_practice (
    user_id    CHAR(16)    NOT NULL,
    day        DATE        NOT NULL,                   -- UTC
    puzzle_id  VARCHAR(12) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    solved     TINYINT(1)  NOT NULL,
    created_at DATETIME(3) NOT NULL,
    PRIMARY KEY (user_id, day, puzzle_id),
    CONSTRAINT fk_trainer_practice_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
