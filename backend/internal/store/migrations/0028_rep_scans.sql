-- The last search of a repertoire for holes (popular replies of the opponent that the tree has no
-- answer to; see internal/repertoire/gaps_popular.go). One row per repertoire, replaced by each
-- search. The start page of the openings shows the figures without asking Lichess again, and the
-- builder shows the saved list at once while a new search runs. moves and max_move_id say which
-- tree the search saw, so that a changed tree marks the row as stale.
CREATE TABLE rep_scans (
    repertoire_id BIGINT      NOT NULL,
    scanned_at    DATETIME(3) NOT NULL,
    moves         INT         NOT NULL,
    max_move_id   BIGINT      NOT NULL,
    missing       INT         NOT NULL,                  -- popular replies the tree lacks
    answer        INT         NOT NULL,                  -- lines that end with the opponent's move
    coverage      DOUBLE      NOT NULL,                  -- 0..1
    partial       TINYINT(1)  NOT NULL,
    source        VARCHAR(10) CHARACTER SET ascii NOT NULL, -- lichess, theory or mixed
    result        MEDIUMTEXT  NOT NULL,                  -- the answer of the search as JSON (the heaviest holes)
    PRIMARY KEY (repertoire_id),
    CONSTRAINT fk_rep_scans_rep FOREIGN KEY (repertoire_id) REFERENCES repertoires (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
