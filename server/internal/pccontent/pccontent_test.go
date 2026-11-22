package pccontent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNothingOpenUntilChosen(t *testing.T) {
	Recheck = 0
	t.Cleanup(func() { SetBanners(nil) })
	dir := t.TempDir()

	SetBanners(nil)
	if !BannerOpen("limited_59") {
		t.Fatal("with no choice in force every summon is open (Bunker's default)")
	}

	Use(dir) // no file yet
	if BannerOpen("limited_59") {
		t.Fatal("a missing file must close every summon")
	}

	if err := Write(dir, Selection{Banners: []string{"limited_59"}}); err != nil {
		t.Fatal(err)
	}
	if !BannerOpen("limited_59") || BannerOpen("chapter_1") {
		t.Fatal("only the chosen summon is open")
	}

	// A broken file closes everything again.
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if BannerOpen("limited_59") {
		t.Fatal("a broken file must close every summon")
	}
}

func TestWriteKeepsEmptyLists(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, Selection{}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"banners\": [],\n  \"events\": []\n}\n"
	if string(raw) != want {
		t.Fatalf("file = %q", raw)
	}
	if Key("", 7) != "gacha_7" || Key("limited_59", 59) != "limited_59" {
		t.Fatal("Key")
	}
	if !AlwaysOpen(3, "chapter_1") || !AlwaysOpen(1, "confirm_ssr") || AlwaysOpen(1, "limited_59") {
		t.Fatal("AlwaysOpen")
	}
}
