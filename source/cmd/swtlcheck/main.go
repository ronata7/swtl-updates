// swtlcheck compares the original game CSVs with the translated CSVs, row by row (matched by ID),
// and reports problems: broken tags/placeholders, empty or untranslated cells, missing rows and
// names translated more than one way. It only reads files and writes a report.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"swtlupdater/internal/resfmt"
)

type table struct {
	header []string
	rows   map[string][]string
	order  []string
	badCol []string // IDs whose column count differs from the header
}

var tokRe = regexp.MustCompile(`(?i)</?[a-z][^>]{0,200}>|\{[0-9]{1,2}\}|%[0-9.]*[a-z]`)

func readTable(path string, t *table) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	first := true
	for sc.Scan() {
		ln := strings.TrimRight(sc.Text(), "\r")
		if first {
			ln = strings.TrimPrefix(ln, "\xEF\xBB\xBF")
		}
		if strings.TrimSpace(ln) == "" {
			continue
		}
		cells := strings.Split(ln, "\t")
		if first {
			first = false
			if t.header == nil {
				t.header = cells
			}
			continue
		}
		id := cells[0]
		if _, dup := t.rows[id]; !dup {
			t.order = append(t.order, id)
		}
		t.rows[id] = cells
		if t.header != nil && len(cells) != len(t.header) && !(len(cells) == len(t.header)-1 && false) {
			t.badCol = append(t.badCol, id)
		}
	}
	return sc.Err()
}

func newTable() *table { return &table{rows: map[string][]string{}} }

func isCJK(r rune) bool {
	return unicode.Is(unicode.Hangul, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Han, r)
}

func plain(s string) string { return tokRe.ReplaceAllString(strings.ReplaceAll(s, "<br>", " "), "") }

func hasCJK(s string) bool {
	for _, r := range plain(s) {
		if isCJK(r) {
			return true
		}
	}
	return false
}

// checkBr: also compare the number of <br> line breaks (off by default, English wraps differently).
var checkBr bool

func tokens(s string) map[string]int {
	m := map[string]int{}
	for _, t := range tokRe.FindAllString(s, -1) {
		t = strings.ToLower(t)
		switch {
		case t == "<br>":
			if !checkBr {
				continue
			}
		case strings.HasPrefix(t, "<font"):
			t = "<font ...>" // the colour is not compared, only that the tag is there
		}
		m[t]++
	}
	return m
}

func diffTokens(a, b map[string]int) string {
	var out []string
	keys := map[string]bool{}
	for k := range a {
		keys[k] = true
	}
	for k := range b {
		keys[k] = true
	}
	var ks []string
	for k := range keys {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	for _, k := range ks {
		if a[k] != b[k] {
			out = append(out, fmt.Sprintf("%s original x%d, translation x%d", k, a[k], b[k]))
		}
	}
	return strings.Join(out, "; ")
}

// filterTok keeps only the placeholders ({0}, %d ...) or only the markup tags.
func filterTok(m map[string]int, placeholders bool) map[string]int {
	r := map[string]int{}
	for k, v := range m {
		if strings.HasPrefix(k, "<") != placeholders {
			r[k] = v
		}
	}
	return r
}

var goodTag = regexp.MustCompile(`(?i)^</?[a-z][^<>]{0,200}>`)

// brokenTag finds a '<' that does not start a complete tag (like "</font<br>") or a stray '>' after text.
func brokenTag(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '<' && i+1 < len(s) && (s[i+1] == '/' || s[i+1]|0x20 >= 'a' && s[i+1]|0x20 <= 'z') && !goodTag.MatchString(s[i:]) {
			e := i + 14
			if e > len(s) {
				e = len(s)
			}
			return "broken tag near: " + s[i:e]
		}
	}
	return ""
}

type issue struct{ file, id, col, kind, detail, orig, tl string }

func clip(s string) string {
	r := []rune(s)
	if len(r) > 80 {
		return string(r[:80]) + "..."
	}
	return s
}

func main() {
	schemaPath := flag.String("schema", "", "schema.json of the game build: checks the FORMAT of every translated CSV against it (what the updater needs)")
	content := flag.Bool("content", false, "also compare the TEXT with the original CSVs (needs -orig; many findings are harmless)")
	origDir := flag.String("orig", "original csv", "folder with the original (untranslated) game CSVs (only used with -content)")
	tlDir := flag.String("tl", "TL csv", "folder with the translated CSVs")
	out := flag.String("out", "check_report.txt", "report file")
	maxShow := flag.Int("max", 40, "most examples shown per problem type and table in the report (the .tsv has all)")
	ratio := flag.Float64("maxratio", 0, "also flag cells where the translation is longer than this many times the original (0 = off)")
	flag.BoolVar(&checkBr, "br", false, "also compare the number of <br> line breaks")
	flag.Parse()

	if *schemaPath != "" {
		code := formatCheck(*schemaPath, *tlDir, *out, *maxShow)
		if !*content {
			os.Exit(code)
		}
		fmt.Println()
		*out = strings.TrimSuffix(*out, filepath.Ext(*out)) + "_content" + filepath.Ext(*out)
	} else if !*content {
		fmt.Println("error: give -schema <schema.json> (format check) and/or -content -orig <folder> (text comparison)")
		os.Exit(1)
	}

	ents, err := os.ReadDir(*origDir)
	if err != nil {
		fmt.Println("error: cannot read the original folder:", err)
		os.Exit(1)
	}
	digitTail := regexp.MustCompile(`\d+$`)
	groups := map[string][]string{} // base name -> original files
	for _, e := range ents {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".csv") {
			continue
		}
		b := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		b = strings.TrimSuffix(b, "_ENG")
		if _, err := os.Stat(filepath.Join(*origDir, b+".csv")); err != nil {
			b = digitTail.ReplaceAllString(b, "")
		}
		groups[b] = append(groups[b], e.Name())
	}
	var bases []string
	for b := range groups {
		bases = append(bases, b)
	}
	sort.Strings(bases)

	var issues []issue
	var rep strings.Builder
	type variants map[string]int
	names := map[string]variants{} // original short text -> translations
	checked := 0
	fmt.Fprintf(&rep, "Translation check\r\nOriginals: %s\r\nTranslations: %s\r\n\r\n", *origDir, *tlDir)

	for _, b := range bases {
		tlPath := ""
		for _, c := range []string{b + ".csv", b + "_ENG.csv"} {
			if _, err := os.Stat(filepath.Join(*tlDir, c)); err == nil {
				tlPath = filepath.Join(*tlDir, c)
				break
			}
		}
		if tlPath == "" {
			continue
		}
		o, t := newTable(), newTable()
		sort.Strings(groups[b])
		okRead := true
		for _, fn := range groups[b] {
			if strings.HasSuffix(strings.TrimSuffix(fn, ".csv"), "_ENG") {
				continue
			}
			if err := readTable(filepath.Join(*origDir, fn), o); err != nil {
				fmt.Fprintf(&rep, "%s: cannot read %s: %v\r\n", b, fn, err)
				okRead = false
			}
		}
		if err := readTable(tlPath, t); err != nil {
			fmt.Fprintf(&rep, "%s: cannot read translation: %v\r\n", b, err)
			okRead = false
		}
		if !okRead || len(o.rows) == 0 {
			continue
		}
		checked++
		var mine []issue
		add := func(id, col, kind, detail, orig, tl string) {
			mine = append(mine, issue{b, id, col, kind, detail, clip(orig), clip(tl)})
		}
		colName := func(i int) string {
			if i < len(t.header) && strings.TrimSpace(t.header[i]) != "" {
				return fmt.Sprintf("%d (%s)", i+1, strings.TrimSpace(t.header[i]))
			}
			return fmt.Sprint(i + 1)
		}
		missing, extra := 0, 0
		for _, id := range o.order {
			oc := o.rows[id]
			tc, ok := t.rows[id]
			if !ok {
				missing++
				continue
			}
			for i := 1; i < len(oc); i++ {
				ov := oc[i]
				if strings.TrimSpace(ov) == "" {
					continue
				}
				tv := ""
				if i < len(tc) {
					tv = tc[i]
				}
				if strings.TrimSpace(tv) == "" {
					add(id, colName(i), "EMPTY", "original has text, translation is empty", ov, tv)
					continue
				}
				if bad := brokenTag(tv); bad != "" && brokenTag(ov) == "" {
					add(id, colName(i), "BROKEN", bad, ov, tv)
				}
				ot, tt := tokens(ov), tokens(tv)
				if d := diffTokens(filterTok(ot, true), filterTok(tt, true)); d != "" {
					add(id, colName(i), "PLACEHOLDER", d, ov, tv)
				}
				if d := diffTokens(filterTok(ot, false), filterTok(tt, false)); d != "" {
					add(id, colName(i), "TAGS", d, ov, tv)
				}
				if hasCJK(tv) {
					add(id, colName(i), "UNTRANSLATED", "still contains Korean/Japanese/Chinese characters", ov, tv)
				}
				if *ratio > 0 {
					lo, lt := len([]rune(plain(ov))), len([]rune(plain(tv)))
					if lo >= 4 && float64(lt) > float64(lo)**ratio {
						add(id, colName(i), "LONG", fmt.Sprintf("%d characters vs %d in the original", lt, lo), ov, tv)
					}
				}
				// names: short plain text translated one way or another
				po := strings.TrimSpace(ov)
				if len([]rune(po)) <= 40 && !strings.Contains(po, "<") && hasCJK(po) && !hasCJK(tv) {
					v := names[po]
					if v == nil {
						v = variants{}
						names[po] = v
					}
					v[strings.TrimSpace(tv)]++
				}
			}
		}
		for _, id := range t.order {
			if _, ok := o.rows[id]; !ok {
				extra++
			}
		}
		for _, id := range t.badCol {
			add(id, "", "COLUMNS", fmt.Sprintf("row has %d columns, header has %d", len(t.rows[id]), len(t.header)), "", "")
		}
		counts := map[string]int{}
		for _, is := range mine {
			counts[is.kind]++
		}
		fmt.Fprintf(&rep, "== %s  (original %d rows, translation %d rows)\r\n", b, len(o.rows), len(t.rows))
		if missing+extra > 0 {
			fmt.Fprintf(&rep, "   %d original ID(s) have no translation row; %d translation row(s) have no original (other game version?)\r\n", missing, extra)
		}
		if len(mine) == 0 {
			fmt.Fprintf(&rep, "   no problems found\r\n\r\n")
			continue
		}
		var kinds []string
		for k := range counts {
			kinds = append(kinds, k)
		}
		sort.Strings(kinds)
		for _, k := range kinds {
			fmt.Fprintf(&rep, "   %s: %d\r\n", k, counts[k])
			shown := 0
			for _, is := range mine {
				if is.kind != k || shown >= *maxShow {
					continue
				}
				shown++
				fmt.Fprintf(&rep, "      ID %s, column %s: %s\r\n", is.id, is.col, is.detail)
				if is.tl != "" {
					fmt.Fprintf(&rep, "         translation: %s\r\n", is.tl)
				}
			}
			if counts[k] > shown {
				fmt.Fprintf(&rep, "      ... and %d more (see the .tsv file)\r\n", counts[k]-shown)
			}
		}
		fmt.Fprintf(&rep, "\r\n")
		issues = append(issues, mine...)
	}

	// names translated more than one way
	type nv struct {
		orig  string
		total int
		vs    []string
	}
	var inc []nv
	for o, v := range names {
		if len(v) < 2 {
			continue
		}
		n := nv{orig: o}
		var ks []string
		top := 0
		for k, c := range v {
			n.total += c
			ks = append(ks, k)
			if c > top {
				top = c
			}
		}
		n.total -= top // rank by how many cells disagree with the most common wording
		sort.Slice(ks, func(i, j int) bool { return v[ks[i]] > v[ks[j]] || v[ks[i]] == v[ks[j]] && ks[i] < ks[j] })
		for _, k := range ks {
			n.vs = append(n.vs, fmt.Sprintf("%q x%d", k, v[k]))
		}
		inc = append(inc, n)
	}
	sort.Slice(inc, func(i, j int) bool { return inc[i].total > inc[j].total })
	fmt.Fprintf(&rep, "== Names or short texts translated more than one way: %d\r\n", len(inc))
	fmt.Fprintf(&rep, "   (same original text, different English; sorted by how many cells use a less common wording. Some are fine, e.g. one word with two meanings)\r\n")
	for i, n := range inc {
		if i >= 150 {
			fmt.Fprintf(&rep, "   ... and %d more\r\n", len(inc)-i)
			break
		}
		fmt.Fprintf(&rep, "   %s  ->  %s\r\n", n.orig, strings.Join(n.vs, ", "))
	}
	for _, n := range inc {
		issues = append(issues, issue{"(all tables)", "", "", "NAME", strings.Join(n.vs, ", "), n.orig, ""})
	}

	if err := os.WriteFile(*out, []byte(rep.String()), 0o644); err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}
	tsv := strings.TrimSuffix(*out, filepath.Ext(*out)) + ".tsv"
	var sb strings.Builder
	sb.WriteString("\xEF\xBB\xBFtable\tID\tcolumn\tproblem\tdetail\toriginal\ttranslation\r\n")
	cl := func(s string) string { return strings.NewReplacer("\t", " ", "\r", " ", "\n", " ").Replace(s) }
	for _, is := range issues {
		fmt.Fprintf(&sb, "%s\t%s\t%s\t%s\t%s\t%s\t%s\r\n", is.file, is.id, cl(is.col), is.kind, cl(is.detail), cl(is.orig), cl(is.tl))
	}
	_ = os.WriteFile(tsv, []byte(sb.String()), 0o644)

	cnt := map[string]int{}
	for _, is := range issues {
		cnt[is.kind]++
	}
	fmt.Printf("Checked %d table(s). Problems found:", checked)
	if len(issues) == 0 {
		fmt.Println(" none")
	} else {
		for _, k := range []string{"BROKEN", "PLACEHOLDER", "TAGS", "EMPTY", "UNTRANSLATED", "COLUMNS", "LONG", "NAME"} {
			if cnt[k] > 0 {
				fmt.Printf("  %s %d", k, cnt[k])
			}
		}
		fmt.Println()
	}
	fmt.Println("Report:", *out)
	fmt.Println("Full list (open in Excel):", tsv)
}

// formatCheck validates every CSV in tlDir against the schema. Returns 2 if any format problem was found.
func formatCheck(schemaPath, tlDir, out string, maxShow int) int {
	sch, err := resfmt.LoadSchema(schemaPath)
	if err != nil {
		fmt.Println("error: cannot read the schema:", err)
		return 1
	}
	ents, err := os.ReadDir(tlDir)
	if err != nil {
		fmt.Println("error: cannot read the translation folder:", err)
		return 1
	}
	var rep strings.Builder
	fmt.Fprintf(&rep, "Format check (against the game's table layout)\r\nTranslations: %s\r\nSchema: %s\r\n\r\n", tlDir, schemaPath)
	var tsv strings.Builder
	tsv.WriteString("\xEF\xBB\xBFfile\tline\tproblem\tdetail\r\n")
	bad, okFiles, skipped := 0, 0, 0
	for _, e := range ents {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".csv") {
			continue
		}
		base := strings.TrimSuffix(strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())), "_ENG")
		fields, ok := sch[strings.ToLower(base)+".res"]
		if !ok {
			skipped++
			fmt.Fprintf(&rep, "== %s\r\n   not a table in this game build's schema - the updater will not use it\r\n\r\n", e.Name())
			continue
		}
		data, err := os.ReadFile(filepath.Join(tlDir, e.Name()))
		if err != nil {
			fmt.Fprintf(&rep, "== %s\r\n   cannot read: %v\r\n\r\n", e.Name(), err)
			bad++
			continue
		}
		probs := resfmt.ValidateCSV(fields, data, 0)
		var warns []string
		for i, ln := range strings.Split(strings.TrimPrefix(string(data), "\xEF\xBB\xBF"), "\n") {
			if i == 0 {
				continue
			}
			cells := strings.Split(strings.TrimSuffix(ln, "\r"), "\t")
			for j, f := range fields {
				if j < len(cells) && strings.EqualFold(f.Type, "string") {
					if b := brokenTag(cells[j]); b != "" {
						warns = append(warns, fmt.Sprintf("line %d, column %d (%s): %s", i+1, j+1, f.Name, b))
					}
				}
			}
		}
		fmt.Fprintf(&rep, "== %s  (%d columns in the table)\r\n", e.Name(), len(fields))
		if len(probs) == 0 && len(warns) == 0 {
			okFiles++
			fmt.Fprintf(&rep, "   format OK\r\n\r\n")
			continue
		}
		show := func(kind string, list []string, note string) {
			if len(list) == 0 {
				return
			}
			fmt.Fprintf(&rep, "   %s: %d  %s\r\n", kind, len(list), note)
			for i, p := range list {
				if i < maxShow {
					fmt.Fprintf(&rep, "      %s\r\n", p)
				}
				fmt.Fprintf(&tsv, "%s\t\t%s\t%s\r\n", e.Name(), kind, strings.ReplaceAll(p, "\t", " "))
			}
			if len(list) > maxShow {
				fmt.Fprintf(&rep, "      ... and %d more (see the .tsv file)\r\n", len(list)-maxShow)
			}
		}
		show("FORMAT", probs, "- the updater cannot build this table until fixed")
		show("BROKEN", warns, "- a cut-off tag such as </font<br>; the table builds, but the text may show wrongly in game")
		fmt.Fprintf(&rep, "\r\n")
		if len(probs) > 0 {
			bad++
		} else {
			okFiles++
		}
	}
	_ = os.WriteFile(out, []byte(rep.String()), 0o644)
	tsvPath := strings.TrimSuffix(out, filepath.Ext(out)) + ".tsv"
	_ = os.WriteFile(tsvPath, []byte(tsv.String()), 0o644)
	fmt.Printf("Format check: %d file(s) OK, %d with FORMAT problems, %d not in the schema.\n", okFiles, bad, skipped)
	fmt.Println("Report:", out)
	fmt.Println("Full list (open in Excel):", tsvPath)
	if bad > 0 {
		return 2
	}
	return 0
}
