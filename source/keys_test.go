package main

import (
	"os"
	"path/filepath"
	"testing"

	"swtlupdater/internal/archivekey"
)

func TestArchivePasswordSources(t *testing.T) { // made-up passwords
	dir := t.TempDir()
	app := &App{dir: dir, cfgPath: filepath.Join(dir, "SWTLUpdater.ini")}
	if pw, _ := app.archivePassword("data12"); pw != "" {
		t.Fatal("a password without any key")
	}
	l1, _ := archivekey.Encode("data12", "readme-pass")
	os.WriteFile(filepath.Join(dir, "README.txt"), []byte("hello\r\n"+l1+"\r\nbye\r\n"), 0o644)
	if pw, _ := app.archivePassword("data12"); pw != "readme-pass" {
		t.Fatalf("README key: %q", pw)
	}
	if pw, _ := app.archivePassword("data11"); pw != "" {
		t.Fatal("key for another data file used")
	}
	l2, _ := archivekey.Encode("data12", "ini-pass")
	os.WriteFile(app.cfgPath, []byte("ArchiveKey="+l2+"\r\n"), 0o644)
	if pw, _ := app.archivePassword("data12"); pw != "ini-pass" {
		t.Fatalf("the settings file must win: %q", pw)
	}
}

func TestArchiveKeySurvivesSettingsSave(t *testing.T) {
	dir := t.TempDir()
	l, _ := archivekey.Encode("data12", "kept-pass")
	c := &Config{ArchiveKey: l, Converter: "builtin"}
	p := filepath.Join(dir, "SWTLUpdater.ini")
	if err := c.Save(p); err != nil {
		t.Fatal(err)
	}
	if back, _ := LoadConfig(dir); back.ArchiveKey != l {
		t.Fatalf("key lost: %q", back.ArchiveKey)
	}
}
