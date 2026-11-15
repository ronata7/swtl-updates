package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

// ScanCSVs checks every *.csv in dir (tab separated, one record per line):
// valid UTF-8, a header, and the same column count on every row.
// It returns the CSV file names and a list of problems.
func ScanCSVs(dir string) (names []string, problems []string) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, []string{err.Error()}
	}
	for _, e := range ents {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".csv") {
			continue
		}
		names = append(names, e.Name())
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", e.Name(), err))
			continue
		}
		if len(data) == 0 {
			problems = append(problems, e.Name()+": file is empty")
			continue
		}
		if !utf8.Valid(data) {
			problems = append(problems, e.Name()+": not valid UTF-8")
			continue
		}
		lines := strings.Split(string(data), "\n")
		header := -1
		bad := 0
		var first []string
		for i, ln := range lines {
			ln = strings.TrimSuffix(ln, "\r")
			if i == 0 {
				ln = strings.TrimPrefix(ln, "\ufeff")
			}
			if ln == "" {
				continue
			}
			n := strings.Count(ln, "\t") + 1
			if header < 0 {
				header = n
				if n < 2 {
					problems = append(problems, e.Name()+": header has no tab separators")
					break
				}
				continue
			}
			if n != header {
				bad++
				if len(first) < 3 {
					first = append(first, fmt.Sprintf("line %d has %d cols", i+1, n))
				}
			}
		}
		if bad > 0 {
			problems = append(problems, fmt.Sprintf("%s: %d row(s) don't match the header's %d columns (%s)", e.Name(), bad, header, strings.Join(first, "; ")))
		}
	}
	sort.Strings(names)
	return
}
