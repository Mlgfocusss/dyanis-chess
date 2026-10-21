-- The open profile (Settings -> Profile): first and last name, a place, a few words about oneself and
-- a country. Every one of them is optional and public (internal/profiles). country is the ISO 3166-1
-- alpha-2 code from internal/countries; it is indexed for the leaderboard of one country and, later,
-- for a table of countries. avatar_url keeps what the person chose to show ("preset:<id>", a Google
-- photo address or nothing for the letter of the name); google_avatar_url remembers the last photo
-- Google sent, so a later sign-in refreshes the photo without overriding a choice.
ALTER TABLE users
    ADD COLUMN first_name        VARCHAR(40)  NULL,
    ADD COLUMN last_name         VARCHAR(40)  NULL,
    ADD COLUMN location          VARCHAR(60)  NULL,
    ADD COLUMN about             VARCHAR(500) NULL,
    ADD COLUMN country           CHAR(2) CHARACTER SET ascii COLLATE ascii_bin NULL,
    ADD COLUMN google_avatar_url VARCHAR(512) NULL,
    ADD KEY idx_users_country (country);

-- Until now the Google photo was the avatar of every Google user.
UPDATE users SET google_avatar_url = avatar_url WHERE kind = 'google' AND avatar_url LIKE 'http%';
