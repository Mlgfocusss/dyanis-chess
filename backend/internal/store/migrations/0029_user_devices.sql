-- What lets the site tell that two accounts are used by one person (see internal/sameperson): the
-- browser (a cookie the site gives to every visitor) and the network address, both kept only as
-- hashes. Rated games, rated matchmaking and duels between accounts that share one of them do not
-- count. kind is 'd' (device cookie) or 'i' (IP address); hash is the first 32 hex digits of a SHA-256.
CREATE TABLE user_devices (
    user_id    CHAR(16)    NOT NULL,
    kind       CHAR(1) CHARACTER SET ascii NOT NULL,
    hash       CHAR(32) CHARACTER SET ascii NOT NULL,
    first_seen DATETIME(3) NOT NULL,
    last_seen  DATETIME(3) NOT NULL,
    PRIMARY KEY (user_id, kind, hash),
    KEY idx_user_devices_hash (kind, hash, last_seen),
    CONSTRAINT fk_user_devices_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
