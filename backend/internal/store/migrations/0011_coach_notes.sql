-- Explanations the AI coach wrote for a move, kept by the hash of what it was asked (the model
-- and the facts), so the same question is never paid for twice, whoever asks and whenever.
CREATE TABLE coach_notes (
    hash       CHAR(64)     CHARACTER SET ascii NOT NULL,
    model      VARCHAR(100) CHARACTER SET ascii NOT NULL,
    text       TEXT         NOT NULL,
    created_at DATETIME(3)  NOT NULL,
    PRIMARY KEY (hash)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
