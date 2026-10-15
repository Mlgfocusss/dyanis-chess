-- Finished game reviews. Queued and running ones live in server memory only.
CREATE TABLE analyses (
    id          CHAR(16)    NOT NULL,
    status      VARCHAR(10) NOT NULL,
    depth       SMALLINT    NOT NULL,
    game        JSON        NOT NULL,           -- api.GameInfo
    report      JSON        NULL,               -- analysis.Report, NULL if the analysis failed
    error       TEXT        NULL,
    created_at  DATETIME(3) NOT NULL,
    finished_at DATETIME(3) NULL,
    PRIMARY KEY (id),
    KEY idx_analyses_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
