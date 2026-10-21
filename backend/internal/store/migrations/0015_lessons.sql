-- Lessons ("Уроки"): how far each person has got in each lesson. The lessons themselves are
-- written in backend/internal/lessons/content and are not in the database. step_index is the
-- number of steps done (the lesson goes on from it), mistakes the wrong moves of the best run,
-- completed_at is set once, when the last step is done.
CREATE TABLE lesson_progress (
    user_id      CHAR(16)    NOT NULL,
    lesson_id    VARCHAR(40) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    step_index   INT         NOT NULL DEFAULT 0,
    mistakes     INT         NOT NULL DEFAULT 0,
    completed_at DATETIME(3) NULL,
    updated_at   DATETIME(3) NOT NULL,
    PRIMARY KEY (user_id, lesson_id),
    CONSTRAINT fk_lesson_progress_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
