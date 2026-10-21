-- The leaderboard reads the best ratings of one category (see internal/profiles); with the category
-- first the index serves "WHERE category = ? ORDER BY rating DESC" without sorting the table.
CREATE INDEX idx_ratings_leaders ON ratings (category, rating);
