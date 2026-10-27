-- The community: friends, private messages, challenges between people and vacations of
-- correspondence games (see internal/community). All of it is tied to the accounts and goes with them.

-- A friendship is two rows, one for each side, so that "my friends" is one indexed read. The side
-- that muted the other's challenges keeps the flag on its own row.
CREATE TABLE friends (
    user_id         CHAR(16)    NOT NULL,
    friend_id       CHAR(16)    NOT NULL,
    mute_challenges TINYINT(1)  NOT NULL DEFAULT 0,   -- user_id does not receive challenges of friend_id
    created_at      DATETIME(3) NOT NULL,
    PRIMARY KEY (user_id, friend_id),
    KEY idx_friends_friend (friend_id),
    CONSTRAINT fk_friends_user   FOREIGN KEY (user_id)   REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT fk_friends_friend FOREIGN KEY (friend_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE friend_requests (
    from_id    CHAR(16)    NOT NULL,
    to_id      CHAR(16)    NOT NULL,
    created_at DATETIME(3) NOT NULL,
    PRIMARY KEY (from_id, to_id),
    KEY idx_friend_requests_to (to_id, created_at),
    CONSTRAINT fk_friend_requests_from FOREIGN KEY (from_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT fk_friend_requests_to   FOREIGN KEY (to_id)   REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- Private messages between friends. read_at is set when the addressee has opened the talk.
CREATE TABLE messages (
    id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    from_id    CHAR(16)        NOT NULL,
    to_id      CHAR(16)        NOT NULL,
    body       VARCHAR(2000)   NOT NULL,
    created_at DATETIME(3)     NOT NULL,
    read_at    DATETIME(3)     NULL,
    PRIMARY KEY (id),
    KEY idx_messages_pair (from_id, to_id, id),
    KEY idx_messages_inbox (to_id, read_at),
    CONSTRAINT fk_messages_from FOREIGN KEY (from_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT fk_messages_to   FOREIGN KEY (to_id)   REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- An invitation to play a game. The game itself is made when the addressee accepts (game_id).
CREATE TABLE challenges (
    id         CHAR(16)    NOT NULL,
    from_id    CHAR(16)    NOT NULL,
    to_id      CHAR(16)    NOT NULL,
    color      VARCHAR(8)  NOT NULL,                 -- the colour of the sender: white, black or random
    base_ms    BIGINT      NOT NULL,
    inc_ms     BIGINT      NOT NULL,
    days       INT         NOT NULL DEFAULT 0,       -- days per move; a correspondence game when above 0
    rated      TINYINT(1)  NOT NULL DEFAULT 0,
    status     VARCHAR(10) NOT NULL,                 -- pending, accepted, declined, cancelled, expired
    game_id    CHAR(16)    NULL,
    created_at DATETIME(3) NOT NULL,
    expires_at DATETIME(3) NOT NULL,
    PRIMARY KEY (id),
    KEY idx_challenges_to (to_id, status, created_at),
    KEY idx_challenges_from (from_id, status, created_at),
    CONSTRAINT fk_challenges_from FOREIGN KEY (from_id) REFERENCES users (id) ON DELETE CASCADE,
    CONSTRAINT fk_challenges_to   FOREIGN KEY (to_id)   REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- Vacation: the clocks of the person's correspondence games do not fall before ends_at.
-- last_ended is when the last vacation stopped, to keep people from living on vacation.
CREATE TABLE vacations (
    user_id    CHAR(16)    NOT NULL,
    ends_at    DATETIME(3) NULL,
    last_ended DATETIME(3) NULL,
    PRIMARY KEY (user_id),
    CONSTRAINT fk_vacations_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
