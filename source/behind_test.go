package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestPlanUpdateServerBehind(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "README.txt"), []byte("new readme"), 0o644)
	os.MkdirAll(filepath.Join(dir, "Tools"), 0o755)
	os.WriteFile(filepath.Join(dir, "Tools", "old.txt"), []byte("x"), 0o644)
	sum := sha256.Sum256([]byte("old readme"))
	m := &Manifest{
		Version: "0.0.1", // older than the running version
		Files:   []ManFile{{Path: "README.txt", Size: 10, SHA256: hex.EncodeToString(sum[:]), Kind: "other"}},
		Delete:  []string{"Tools/old.txt"},
	}
	app := &App{dir: dir, cfg: &Config{}}
	plan, err := app.planUpdate(m, filepath.Join(dir, "SWTLUpdater.exe"), updSource{label: "Program", seen: "update_seen.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Behind || len(plan.Download) != 0 || len(plan.Delete) != 0 {
		t.Fatalf("an older published version must not change anything: %+v", plan)
	}
	// the same version with different files is still a normal "files changed" update
	m.Version = version
	plan, err = app.planUpdate(m, filepath.Join(dir, "SWTLUpdater.exe"), updSource{label: "Program", seen: "update_seen.txt"})
	if err != nil || plan.Behind || len(plan.Download) != 1 {
		t.Fatalf("same version should still update changed files: %+v %v", plan, err)
	}
}
