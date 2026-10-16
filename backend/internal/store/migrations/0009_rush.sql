-- Tactics storm ("Тактический штурм"): every finished run of a person, to show their best.
-- Practice only: it does not move the puzzle rating.
CREATE TABLE rush_results (
    id         BIGINT      NOT NULL AUTO_INCREMENT,
    user_id    CHAR(16)    NOT NULL,
    mode       VARCHAR(16) CHARACTER SET ascii NOT NULL,   -- "3min", "5min"
    score      INT         NOT NULL,                       -- puzzles solved
    created_at DATETIME(3) NOT NULL,
    PRIMARY KEY (id),
    KEY idx_rush_user_mode (user_id, mode, score),
    CONSTRAINT fk_rush_results_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
