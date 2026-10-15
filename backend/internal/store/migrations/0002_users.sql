-- Accounts: guests (created with one request) and Google sign-in. A guest
-- becomes a "google" user in place when they sign in, keeping their games.
CREATE TABLE users (
    id           CHAR(16)     NOT NULL,
    kind         VARCHAR(10)  NOT NULL,          -- 'guest' or 'google'
    google_sub   VARCHAR(64)  NULL,              -- Google's stable account id
    name         VARCHAR(100) NOT NULL,
    email        VARCHAR(255) NULL,
    avatar_url   VARCHAR(512) NULL,
    settings     JSON         NULL,              -- the settings page, opaque here
    created_at   DATETIME(3)  NOT NULL,
    last_seen_at DATETIME(3)  NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uq_users_google_sub (google_sub)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- Browser sessions. Only a hash of the cookie's token is stored, so a copy
-- of the database cannot be used to sign in.
CREATE TABLE sessions (
    token_hash CHAR(64)    NOT NULL,             -- hex SHA-256 of the cookie value
    user_id    CHAR(16)    NOT NULL,
    created_at DATETIME(3) NOT NULL,
    expires_at DATETIME(3) NOT NULL,
    PRIMARY KEY (token_hash),
    KEY idx_sessions_user (user_id),
    KEY idx_sessions_expires (expires_at),
    CONSTRAINT fk_sessions_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
