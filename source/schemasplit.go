package main

import (
	"bytes"
	"encoding/json"
	"os"
	"sort"
	"strings"
)

// splitLocaleSchema gives every language table that is really in the dump (tb_X_ENG.res, _TWN, _CHN) its own
// entry in schema.json, a copy of the entry of tb_X.res. Our tools look tables up without the suffix and do not
// need this; older tools (SWAdmin CLI) look up the exact file name and do.
func splitLocaleSchema(path string, files []string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var es []resSchemaEntry
	if err := json.Unmarshal(bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF}), &es); err != nil {
		return err
	}
	have := map[string]bool{}
	for _, e := range es {
		have[strings.ToLower(e.File)] = true
	}
	extra := map[string][]resSchemaEntry{} // base key -> new entries
	seen := map[string]bool{}
	sorted := append([]string(nil), files...)
	sort.Strings(sorted)
	for _, n := range sorted {
		ln := strings.ToLower(n)
		key := schemaKey(n)
		if key == ln || have[ln] || seen[ln] {
			continue
		}
		seen[ln] = true
		for _, e := range es {
			if strings.ToLower(e.File) == key {
				extra[key] = append(extra[key], resSchemaEntry{File: n, Fields: e.Fields, Inferred: e.Inferred})
				break
			}
		}
	}
	if len(extra) == 0 {
		return nil
	}
	var out []resSchemaEntry
	for _, e := range es {
		out = append(out, e)
		out = append(out, extra[strings.ToLower(e.File)]...)
	}
	j, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, j, 0o644)
}
