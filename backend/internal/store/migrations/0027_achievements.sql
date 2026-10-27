-- Achievements ("Достижения"): the badges a person has earned. What earns a badge is computed from the
-- other tables on reading (see internal/achievements); this table only remembers that it was earned and
-- when, so the date does not move and a badge does not vanish when the thing it was earned by changes.
-- seen = 0 until the person's browser has shown the "new achievement" notice.
CREATE TABLE achievements (
    user_id     CHAR(16)    NOT NULL,
    ach_key     VARCHAR(40) CHARACTER SET ascii NOT NULL,
    unlocked_at DATETIME(3) NOT NULL,
    seen        TINYINT(1)  NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, ach_key),
    KEY idx_achievements_time (user_id, unlocked_at),
    CONSTRAINT fk_achievements_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
