package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// optional: game tables and schema from a local folder (not part of the repository)
func TestReadResRealGameTables(t *testing.T) {
	dir := os.Getenv("SWTL_REAL_RES")
	if dir == "" {
		t.Skip("set SWTL_REAL_RES=<folder with .res and schema.json>")
	}
	sch, err := loadResSchema(filepath.Join(dir, "schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	ms, _ := filepath.Glob(filepath.Join(dir, "*.res"))
	for _, f := range ms {
		b, _ := os.ReadFile(f)
		fl := sch[filepath.Base(f)]
		if fl == nil {
			fl = sch[lower(filepath.Base(f))]
		}
		rows, err := readRes(fl, b)
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		if !bytes.Equal(buildRes(rows), b) {
			t.Errorf("%s: rebuilt table differs from the game's (%d rows)", f, len(rows))
		} else {
			t.Logf("%s: %d rows read and rebuilt byte-exact", filepath.Base(f), len(rows))
		}
	}
}

func lower(s string) string { return string(bytes.ToLower([]byte(s))) }
