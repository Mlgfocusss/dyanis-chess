-- Bughouse (Swedish chess) has four players: the seats 3 and 4 of a variant game. White and Black keep the first
-- two seats (board 1 White, board 1 Black); third_id and fourth_id are board 2 White and board 2 Black. NULL for
-- every other variant and while a seat is empty.
ALTER TABLE variant_games
    ADD COLUMN third_id  CHAR(16) NULL AFTER black_id,
    ADD COLUMN fourth_id CHAR(16) NULL AFTER third_id,
    ADD KEY idx_variant_games_third (third_id, updated_at),
    ADD KEY idx_variant_games_fourth (fourth_id, updated_at),
    ADD CONSTRAINT fk_variant_games_third FOREIGN KEY (third_id) REFERENCES users (id) ON DELETE SET NULL,
    ADD CONSTRAINT fk_variant_games_fourth FOREIGN KEY (fourth_id) REFERENCES users (id) ON DELETE SET NULL;
