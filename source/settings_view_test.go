package main

import (
	"strings"
	"testing"
)

func TestSettingRowsSyncedAndNumbered(t *testing.T) {
	a := &App{cfg: defaultConfig(t.TempDir())}
	rows := a.settingRows()
	if len(rows) != 13 {
		t.Fatalf("rows: %d", len(rows))
	}
	for i, r := range rows {
		if r.n != strings.TrimSpace(strings.TrimLeft(strings.Fields(r.n)[0], " ")) || r.label == "" {
			t.Fatalf("bad row %v", r)
		}
		if len(r.label) > 29 {
			t.Fatalf("label too long for the dots: %q", r.label)
		}
		_ = i
	}
}
