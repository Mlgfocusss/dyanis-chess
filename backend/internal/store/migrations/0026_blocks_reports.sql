-- Blocking a player and reports to the moderators (see internal/community and internal/forum).

-- blocker_id does not want to hear from blocked_id: no messages, friend requests or challenges either
-- way. Both ends go with their account.
CREATE TABLE blocks (
    blocker_id CHAR(16)    NOT NULL,
    blocked_id CHAR(16)    NOT NULL,
    created_at DATETIME(3) NOT NULL,
    PRIMARY KEY (blocker_id, blocked_id),
    KEY idx_blocks_blocked (blocked_id),
    CONSTRAINT fk_blocks_blocker FOREIGN KEY (blocker_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT fk_blocks_blocked FOREIGN KEY (blocked_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- A complaint. The text complained about is copied here (snapshot), so deleting it does not erase
-- the evidence. The people are kept by name as well: the report outlives the accounts (SET NULL).
-- open_key is 1 while the report is open and NULL after, so that one person can have only one open
-- report on one target (the unique key ignores NULLs).
CREATE TABLE reports (
    id             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    reporter_id    CHAR(16)        NULL,
    reporter_name  VARCHAR(100)    NOT NULL,
    target_kind    ENUM('message', 'forum_post', 'forum_topic', 'user') NOT NULL,
    target_id      VARCHAR(32) CHARACTER SET ascii NOT NULL,
    target_user_id CHAR(16)        NULL,
    target_name    VARCHAR(100)    NOT NULL DEFAULT '',
    topic_id       BIGINT UNSIGNED NULL,
    reason         ENUM('spam', 'abuse', 'cheating', 'other') NOT NULL,
    note           VARCHAR(300)    NOT NULL DEFAULT '',
    snapshot       TEXT            NOT NULL,
    status         ENUM('open', 'closed') NOT NULL DEFAULT 'open',
    open_key       TINYINT         NULL DEFAULT 1,
    action         VARCHAR(16)     NOT NULL DEFAULT '',
    created_at     DATETIME(3)     NOT NULL,
    closed_by      CHAR(16)        NULL,
    closed_at      DATETIME(3)     NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uq_reports_open (reporter_id, target_kind, target_id, open_key),
    KEY idx_reports_status (status, created_at),
    KEY idx_reports_target (target_kind, target_id),
    CONSTRAINT fk_reports_reporter FOREIGN KEY (reporter_id)    REFERENCES users (id) ON DELETE SET NULL,
    CONSTRAINT fk_reports_target   FOREIGN KEY (target_user_id) REFERENCES users (id) ON DELETE SET NULL,
    CONSTRAINT fk_reports_closer   FOREIGN KEY (closed_by)      REFERENCES users (id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
