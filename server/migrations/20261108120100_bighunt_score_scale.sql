-- Subjugation scores used the total damage as the base score; the game's base score is
-- the damage / 100 (its grade table: X at 500,000,000). Bring the stored high scores to
-- that scale once, so new runs can beat them.
-- +goose Up
UPDATE user_big_hunt_max_scores SET max_score = max_score / 100;
UPDATE user_big_hunt_schedule_max_scores SET max_score = max_score / 100;
UPDATE user_big_hunt_weekly_max_scores SET max_score = max_score / 100;

-- +goose Down
UPDATE user_big_hunt_max_scores SET max_score = max_score * 100;
UPDATE user_big_hunt_schedule_max_scores SET max_score = max_score * 100;
UPDATE user_big_hunt_weekly_max_scores SET max_score = max_score * 100;
