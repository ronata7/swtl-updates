package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Full export: EVERY table of a game install as .res, plus a readable .csv of each table that is checked to
// convert back to exactly the same .res (a lossless round trip). The TL-only reference export (swtl-reference)
// is unchanged; this one is bigger and meant for a separate, private place.

// fullSelect puts every table in a group: gb tables with a language suffix go to <LANG>-res, the rest to res.
func fullSelect(region string, names []string) []exportGroup {
	if region != "gb" {
		return []exportGroup{{dir: "res", files: append([]string(nil), names...)}}
	}
	by := map[string]*exportGroup{}
	var order []string
	for _, n := range names {
		dir := "res"
		ln := strings.ToLower(n)
		for _, lang := range []string{"ENG", "TWN", "CHN"} {
			if strings.HasSuffix(ln, "_"+strings.ToLower(lang)+".res") {
				dir = lang + "-res"
			}
		}
		g := by[dir]
		if g == nil {
			g = &exportGroup{dir: dir}
			by[dir] = g
			order = append(order, dir)
		}
		g.files = append(g.files, n)
	}
	sort.Strings(order)
	var out []exportGroup
	for _, d := range order {
		out = append(out, *by[d])
	}
	return out
}

// schemaKey turns a game table file name (maybe with _ENG/_TWN/_CHN) into the key used in schema.json.
func schemaKey(name string) string {
	ln := strings.ToLower(name)
	for _, s := range []string{"_eng.res", "_twn.res", "_chn.res"} {
		if strings.HasSuffix(ln, s) {
			return strings.TrimSuffix(ln, s) + ".res"
		}
	}
	return ln
}

// resToCSV writes the readable csv of a table (same layout as the translation csv files: a header with the
// column names, tab separated, CRLF, <br> for line breaks).
func resToCSV(fields []resField, rows []resRow) []byte {
	var b bytes.Buffer
	for _, f := range fields {
		b.WriteString(f.Name)
		b.WriteByte('\t')
	}
	b.WriteString("\r\n")
	for _, r := range rows {
		b.WriteString(rowCSVLineMode(fields, r, true))
		b.WriteString("\t\r\n")
	}
	return b.Bytes()
}

// roundTrip converts the csv back to a table and compares it with the original bytes.
// It returns "" when identical, else a short reason.
func roundTrip(fields []resField, csv, orig []byte) string {
	back, err := csvToResMode(fields, csv, true)
	if err != nil {
		return "csv does not convert back: " + err.Error()
	}
	if bytes.Equal(back, orig) {
		return ""
	}
	a, e1 := readRes(fields, orig)
	c, e2 := readRes(fields, back)
	if e1 != nil || e2 != nil {
		return "converted table differs and cannot be compared"
	}
	if len(a) != len(c) {
		return fmt.Sprintf("row count differs (%d -> %d): text with a tab or lone line break?", len(a), len(c))
	}
	for i := range a {
		if !bytes.Equal(a[i].raw, c[i].raw) {
			return fmt.Sprintf("first difference at data row %d (ID %s)", i+1, strings.SplitN(rowCSVLineMode(fields, a[i], true), "\t", 2)[0])
		}
	}
	return "only the closing checksum differs"
}

type csvResult struct {
	file, status string
	rows, cols   int
}

// writeCSVs makes csv/<name>.csv beside the res folder and returns one result line per table.
func writeCSVs(schema map[string][]resField, resDir, csvDir string, files []string) ([]csvResult, error) {
	if err := os.MkdirAll(csvDir, 0o755); err != nil {
		return nil, err
	}
	var out []csvResult
	for _, n := range files {
		orig, err := os.ReadFile(filepath.Join(resDir, n))
		if err != nil {
			return nil, err
		}
		fields, ok := schema[schemaKey(n)]
		if !ok {
			out = append(out, csvResult{file: n, status: "NO SCHEMA - res only"})
			continue
		}
		rows, err := readRes(fields, orig)
		if err != nil {
			out = append(out, csvResult{file: n, cols: len(fields), status: "does not fit the schema - res only (" + err.Error() + ")"})
			continue
		}
		csv := resToCSV(fields, rows)
		name := strings.TrimSuffix(n, filepath.Ext(n)) + ".csv"
		if err := os.WriteFile(filepath.Join(csvDir, name), csv, 0o644); err != nil {
			return nil, err
		}
		st := "ROUND-TRIP OK"
		if why := roundTrip(fields, csv, orig); why != "" {
			st = "NOT LOSSLESS - " + why + " (keep the .res as the master)"
		}
		out = append(out, csvResult{file: n, rows: len(rows), cols: len(fields), status: st})
	}
	return out, nil
}

// The full dump csv must convert back to the very same table, so its text cells use a small, reversible set of
// markers (the same <br> for a line break as the translation csv files, so normal text looks identical):
//
//	line feed -> <br>      carriage return -> <cr>      tab -> <tab>
//	a text that really contains the characters  <br>  <cr>  <tab>  or  <esc>  gets <esc> in front of that tag.
var dumpTags = []string{"<br>", "<cr>", "<tab>", "<esc>"}

func escapeDump(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		switch s[i] {
		case '\n':
			b.WriteString("<br>")
			i++
			continue
		case '\r':
			b.WriteString("<cr>")
			i++
			continue
		case '\t':
			b.WriteString("<tab>")
			i++
			continue
		case '<':
			hit := false
			for _, t := range dumpTags {
				if strings.HasPrefix(s[i:], t) {
					b.WriteString("<esc>" + t)
					i += len(t)
					hit = true
					break
				}
			}
			if hit {
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func unescapeDump(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '<' {
			if strings.HasPrefix(s[i:], "<esc>") {
				done := false
				for _, t := range dumpTags {
					if strings.HasPrefix(s[i+5:], t) {
						b.WriteString(t)
						i += 5 + len(t)
						done = true
						break
					}
				}
				if done {
					continue
				}
			}
			switch {
			case strings.HasPrefix(s[i:], "<br>"):
				b.WriteByte('\n')
				i += 4
				continue
			case strings.HasPrefix(s[i:], "<cr>"):
				b.WriteByte('\r')
				i += 4
				continue
			case strings.HasPrefix(s[i:], "<tab>"):
				b.WriteByte('\t')
				i += 5
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// DumpCSVToRes converts the csv files of a full dump back to .res tables (lossless markers), one table per
// csv, using the dump's own schema.json. If compareDir holds the original .res files, every result is compared
// byte for byte and reported. Returns 0 when everything was converted (and matched, if compared).
func (a *App) DumpCSVToRes(schemaPath, csvDir, outDir, compareDir string) int {
	log := &Logger{}
	sm, err := loadResSchema(schemaPath)
	if err != nil {
		log.Fail("cannot read the schema %s: %v", schemaPath, err)
		return 1
	}
	files, _ := filepath.Glob(filepath.Join(csvDir, "*.csv"))
	if len(files) == 0 {
		log.Fail("no .csv files in %s", csvDir)
		return 1
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Fail("%v", err)
		return 1
	}
	bad := 0
	for _, f := range files {
		base := strings.TrimSuffix(filepath.Base(f), ".csv")
		fields, ok := sm[schemaKey(base+".res")]
		if !ok {
			log.Warn("%s: no schema entry - skipped", base)
			bad++
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			log.Fail("%v", err)
			bad++
			continue
		}
		out, err := csvToResMode(fields, b, true)
		if err != nil {
			log.Fail("%s: %v", base, err)
			bad++
			continue
		}
		if err := os.WriteFile(filepath.Join(outDir, base+".res"), out, 0o644); err != nil {
			log.Fail("%v", err)
			bad++
			continue
		}
		if compareDir != "" {
			orig, err := os.ReadFile(filepath.Join(compareDir, base+".res"))
			switch {
			case err != nil:
				log.Warn("%s: no original to compare", base)
			case !bytes.Equal(orig, out):
				log.Fail("%s: DIFFERENT from the original table", base)
				bad++
			default:
				log.OK("%s: identical to the original", base)
			}
		} else {
			log.OK("%s: converted", base)
		}
	}
	if bad > 0 {
		log.Fail("%d table(s) need attention", bad)
		return 1
	}
	log.OK("All %d table(s) converted into %s", len(files), outDir)
	return 0
}
