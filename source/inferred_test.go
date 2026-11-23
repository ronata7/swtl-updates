package main

import (
	"os"
	"path/filepath"
	"testing"
)

// SWTL_INFER_DIR = a folder with sub folders of real .res files (tables the game's schema lacks or does not fit).
// Every table that has an inferred layout must convert to csv and back to the same bytes.
func TestInferredLayoutsRoundTrip(t *testing.T) {
	dir := os.Getenv("SWTL_INFER_DIR")
	if dir == "" {
		t.Skip("SWTL_INFER_DIR not set")
	}
	inf := inferredSchema()
	files, _ := filepath.Glob(filepath.Join(dir, "*", "*.res"))
	seen := map[string]int{}
	for _, f := range files {
		orig, _ := os.ReadFile(f)
		if len(orig) <= 38 {
			continue // an empty stub (row count but no rows): nothing to check
		}
		key := schemaKey(filepath.Base(f))
		cands, ok := inf[key]
		if !ok {
			continue
		}
		good := false
		for _, fields := range cands {
			if rows, err := readRes(fields, orig); err == nil && roundTrip(fields, resToCSV(fields, rows), orig) == "" {
				good = true
			}
		}
		if !good {
			t.Errorf("%s: no inferred layout round-trips", f)
			continue
		}
		seen[key]++
	}
	for k := range inf {
		if seen[k] == 0 {
			t.Errorf("no sample file checked for %s", k)
		}
	}
	t.Logf("checked %v", seen)
}
