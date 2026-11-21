package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSessionLogHidesSecrets(t *testing.T) {
	dir := t.TempDir()
	openSession(dir)
	defer func() { sess.mu.Lock(); sess.f.Close(); sess.f = nil; sess.mu.Unlock() }()
	a := &Config{SWPath: `C:\game`, BackupKeep: 5, UpdateKey: "SECRETUPDATEKEY123", ArchiveKey: "SWTLKEY1:data12:SECRETARCHIVE"}
	b := copyConfig(a)
	b.BackupKeep = 8
	b.UpdateKey = "ANOTHERSECRETKEY456"
	b.ArchiveKey = ""
	for _, l := range configDiff(a, b) {
		sessf("setting changed: %s", l)
	}
	for _, l := range configDump(a) {
		sessf("  setting %s", l)
	}
	sessf("input: %q -> (hidden)", "Update key (paste it)")
	if !secretPrompt("  Update key (paste it, Enter = keep): ") || secretPrompt(" > ") {
		t.Fatal("secretPrompt wrong")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "session_*.log"))
	if len(files) != 1 {
		t.Fatalf("session logs: %v", files)
	}
	data, _ := os.ReadFile(files[0])
	txt := string(data)
	for _, bad := range []string{"SECRETUPDATEKEY123", "ANOTHERSECRETKEY456", "SECRETARCHIVE"} {
		if strings.Contains(txt, bad) {
			t.Fatalf("secret %q leaked into the log:\n%s", bad, txt)
		}
	}
	if !strings.Contains(txt, "BackupKeep: 5 -> 8") || !strings.Contains(txt, "UpdateKey: changed") || !strings.Contains(txt, "ArchiveKey: changed") {
		t.Fatalf("missing change lines:\n%s", txt)
	}
}
