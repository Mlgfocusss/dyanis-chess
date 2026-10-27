-- The forum (see internal/forum). A post stays when its author's account goes: author_id becomes NULL
-- and the page says "Удалённый пользователь"; author_name is the name at the time of writing.
CREATE TABLE forum_topics (
    id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    category     VARCHAR(16) CHARACTER SET ascii NOT NULL,
    author_id    CHAR(16)        NULL,
    author_name  VARCHAR(100)    NOT NULL,
    title        VARCHAR(160)    NOT NULL,
    created_at   DATETIME(3)     NOT NULL,
    last_post_at DATETIME(3)     NOT NULL,
    posts        INT             NOT NULL DEFAULT 1,
    pinned       TINYINT(1)      NOT NULL DEFAULT 0,
    locked       TINYINT(1)      NOT NULL DEFAULT 0,
    PRIMARY KEY (id),
    KEY idx_forum_topics_list (category, pinned, last_post_at),
    KEY idx_forum_topics_author (author_id),
    CONSTRAINT fk_forum_topics_author FOREIGN KEY (author_id) REFERENCES users (id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE forum_posts (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    topic_id    BIGINT UNSIGNED NOT NULL,
    author_id   CHAR(16)        NULL,
    author_name VARCHAR(100)    NOT NULL,
    body        TEXT            NOT NULL,
    created_at  DATETIME(3)     NOT NULL,
    edited_at   DATETIME(3)     NULL,
    deleted     TINYINT(1)      NOT NULL DEFAULT 0,
    PRIMARY KEY (id),
    KEY idx_forum_posts_topic (topic_id, id),
    KEY idx_forum_posts_author (author_id, created_at),
    CONSTRAINT fk_forum_posts_topic  FOREIGN KEY (topic_id)  REFERENCES forum_topics (id) ON DELETE CASCADE,
    CONSTRAINT fk_forum_posts_author FOREIGN KEY (author_id) REFERENCES users (id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
