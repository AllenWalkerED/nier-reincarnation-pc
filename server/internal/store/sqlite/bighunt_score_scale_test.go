package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/pressly/goose/v3"

	"lunar-tear/server/internal/database"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/migrations"
)

// Subjugation high scores saved before the base score became damage / 100 come down to
// the new scale once, when the save is first opened by this version.
func TestSubjugationHighScoresAreRescaled(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(filepath.Join(t.TempDir(), "game.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := migrations.Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	uid, err := New(db, time.Now).CreateUser("test-uuid", model.ClientPlatform{})
	if err != nil {
		t.Fatal(err)
	}
	// A save from before the rescale: the scale migration has not run yet.
	if err := goose.DownToContext(ctx, db, ".", 20261108120000); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO user_big_hunt_max_scores (user_id, big_hunt_boss_id, max_score) VALUES (?, 1, 1638936907)`,
		`INSERT INTO user_big_hunt_schedule_max_scores (user_id, big_hunt_schedule_id, big_hunt_boss_id, max_score) VALUES (?, 1, 1, 1638936907)`,
		`INSERT INTO user_big_hunt_weekly_max_scores (user_id, big_hunt_weekly_version, attribute_type, max_score) VALUES (?, 1, 2, 1638936907)`,
	} {
		if _, err := db.Exec(q, uid); err != nil {
			t.Fatal(err)
		}
	}
	if err := migrations.Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"user_big_hunt_max_scores", "user_big_hunt_schedule_max_scores", "user_big_hunt_weekly_max_scores"} {
		var got int64
		if err := db.QueryRow(`SELECT max_score FROM `+table+` WHERE user_id = ?`, uid).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != 16389369 {
			t.Errorf("%s max_score = %d, want 16389369", table, got)
		}
	}
	// Opening the save again leaves the scores alone.
	if err := migrations.Up(ctx, db); err != nil {
		t.Fatal(err)
	}
	var again int64
	db.QueryRow(`SELECT max_score FROM user_big_hunt_max_scores WHERE user_id = ?`, uid).Scan(&again)
	if again != 16389369 {
		t.Errorf("second open: max_score = %d, want 16389369", again)
	}
}
