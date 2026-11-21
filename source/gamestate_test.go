package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGameChangedNotice(t *testing.T) {
	root := t.TempDir()
	game := filepath.Join(root, "game")
	os.MkdirAll(filepath.Join(game, "datas"), 0o755)
	app := &App{dir: filepath.Join(root, "app"), cfg: &Config{SWPath: game, DataSource: "data12"}}
	write := func(name, s string) { os.WriteFile(filepath.Join(game, name), []byte(s), 0o644) }
	write("SoulWorker64.dll", "dll1")
	write(filepath.Join("datas", "data12.v"), "data1")
	if app.gameChangedNotice() != "" {
		t.Fatal("a notice before anything was recorded")
	}
	src, _ := hashFile(app.gameDataFile())
	app.saveGameState(src, "outputhash")
	if app.gameChangedNotice() != "" {
		t.Fatal("a notice although nothing changed")
	}
	// the patched file was installed in the game folder: still fine
	write(filepath.Join("datas", "data12.v"), "patched")
	h, _ := hashFile(app.gameDataFile())
	app.saveGameState(src, h)
	if app.gameChangedNotice() != "" {
		t.Fatal("a notice for the updater's own output")
	}
	// the data file is replaced by something else
	write(filepath.Join("datas", "data12.v"), "other")
	if m := app.gameChangedNotice(); !strings.Contains(m, "not the file") {
		t.Fatalf("replaced file: %q", m)
	}
	// the game was updated: dll and data both change
	write("SoulWorker64.dll", "dll2")
	if m := app.gameChangedNotice(); !strings.Contains(m, "game was updated") {
		t.Fatalf("game update: %q", m)
	}
}

func TestIsOurPatchedFile(t *testing.T) {
	app := &App{dir: t.TempDir(), cfg: &Config{}}
	if app.isOurPatchedFile("abc") {
		t.Fatal("no record yet: must be false")
	}
	app.saveGameState("src111", "out222")
	if !app.isOurPatchedFile("out222") || !app.isOurPatchedFile("OUT222") {
		t.Fatal("the patched file must be recognised")
	}
	if app.isOurPatchedFile("src111") || app.isOurPatchedFile("") {
		t.Fatal("the original file must not be flagged")
	}
}
