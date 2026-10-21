-- Statistics tab (internal/stats): the days with puzzles are counted from the time of each row of a
-- person, and rush_results had no index that ends in the time (the others already have one).
ALTER TABLE rush_results ADD KEY idx_rush_user_time (user_id, created_at);
