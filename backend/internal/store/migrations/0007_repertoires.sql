-- Opening repertoires: a person's own tree of lines, and the memorising of it.
-- A repertoire is for one colour. Every move of the tree is a row; the moves
-- the person has to play themselves ("mine") are also the cards of the spaced
-- repetition, so the card's state lives on the same row.
CREATE TABLE repertoires (
    id         BIGINT       NOT NULL AUTO_INCREMENT,
    user_id    CHAR(16)     NOT NULL,
    name       VARCHAR(80)  NOT NULL,
    color      ENUM('white', 'black') NOT NULL,
    created_at DATETIME(3)  NOT NULL,
    PRIMARY KEY (id),
    KEY idx_repertoires_user (user_id),
    CONSTRAINT fk_repertoires_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE rep_moves (
    id            BIGINT       NOT NULL AUTO_INCREMENT,
    repertoire_id BIGINT       NOT NULL,
    parent_id     BIGINT       NOT NULL DEFAULT 0,        -- 0: a first move from the start position
    uci           VARCHAR(5)   NOT NULL,
    san           VARCHAR(12)  NOT NULL,
    fen           VARCHAR(100) NOT NULL,                  -- the position after the move
    ply           INT          NOT NULL,                  -- 1 for the first move of White
    mine          TINYINT(1)   NOT NULL,                  -- the person's own move (a card)
    box           INT          NOT NULL DEFAULT 0,        -- steps of the ladder passed
    due_at        DATETIME(3)  NULL,                      -- NULL: never asked yet
    lapses        INT          NOT NULL DEFAULT 0,
    reviews       INT          NOT NULL DEFAULT 0,
    reviewed_at   DATETIME(3)  NULL,
    created_at    DATETIME(3)  NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uq_rep_moves_edge (repertoire_id, parent_id, uci),
    KEY idx_rep_moves_due (repertoire_id, mine, due_at),
    CONSTRAINT fk_rep_moves_rep FOREIGN KEY (repertoire_id) REFERENCES repertoires (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
