package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestKeepNewest(t *testing.T) {
	d := t.TempDir()
	for i := 0; i < 8; i++ {
		p := filepath.Join(d, fmt.Sprintf("update_%d.log", i))
		os.WriteFile(p, []byte("x"), 0o644)
		tm := time.Now().Add(time.Duration(i) * time.Minute)
		os.Chtimes(p, tm, tm)
	}
	os.WriteFile(filepath.Join(d, "other.txt"), []byte("x"), 0o644)
	if n := keepNewest(d, "update_*.log", 3); n != 5 {
		t.Fatalf("removed %d", n)
	}
	left, _ := filepath.Glob(filepath.Join(d, "update_*.log"))
	if len(left) != 3 || !exists(filepath.Join(d, "update_7.log")) || exists(filepath.Join(d, "update_0.log")) || !exists(filepath.Join(d, "other.txt")) {
		t.Fatal("wrong files kept", left)
	}
	if keepNewest(d, "update_*.log", 0) != 0 {
		t.Fatal("0 must keep all")
	}
}
