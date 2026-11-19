package main

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Built-in replacement for "SWAdmin.Cli.exe -csv_to_res" (SWAdmin tools, see credits).
//
// Table (.res) layout, worked out from sample tables:
//   uint32  number of rows
//   rows    each field in schema order: byte/uint16/uint32/int16/int32 little endian,
//           string = uint16 length in UTF-16 units + UTF-16LE characters
//   footer  uint16 32 + 32 ASCII hex characters = MD5 of the decimal text of a checksum number, where the
//           number is the sum over all fields: a number field adds its value, a string field adds its length
//           plus the sum of the bytes of its UTF-16LE text.
// In the CSV a line break inside a text is written as "<br>".

type resField struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type resSchemaEntry struct {
	File   string     `json:"file"`
	Fields []resField `json:"fields"`
}

// loadResSchema reads schema.json (as made by SWAdmin -dumper_to_schema) keyed by lower-case table file name.
func loadResSchema(path string) (map[string][]resField, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var es []resSchemaEntry
	if err := json.Unmarshal(bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF}), &es); err != nil {
		return nil, fmt.Errorf("schema file is not valid: %v", err)
	}
	m := map[string][]resField{}
	for _, e := range es {
		if e.File == "" {
			return nil, fmt.Errorf("schema has an entry without a file name")
		}
		m[strings.ToLower(e.File)] = e.Fields
	}
	return m, nil
}

// resRow is one encoded table row: its bytes, its share of the table checksum, and a key (the bytes of
// its first column) used to match a row of the translation with the same row of the game's own table.
type resRow struct {
	raw []byte
	sum int64
	key string
}

// csvToRes converts the text of one translation CSV into the binary table.
func csvToRes(fields []resField, csv []byte) ([]byte, error) {
	rows, err := encodeCSV(fields, csv)
	if err != nil {
		return nil, err
	}
	return buildRes(rows), nil
}

func buildRes(rows []resRow) []byte {
	var out bytes.Buffer
	binary.Write(&out, binary.LittleEndian, uint32(len(rows)))
	var sum int64
	for _, r := range rows {
		out.Write(r.raw)
		sum += r.sum
	}
	h := md5.Sum([]byte(strconv.FormatInt(sum, 10)))
	hx := hex.EncodeToString(h[:])
	binary.Write(&out, binary.LittleEndian, uint16(len(hx)))
	out.WriteString(hx)
	return out.Bytes()
}

func encodeCSV(fields []resField, csv []byte) ([]resRow, error) {
	csv = bytes.TrimPrefix(csv, []byte{0xEF, 0xBB, 0xBF})
	if len(bytes.TrimSpace(csv)) == 0 {
		return nil, fmt.Errorf("CSV file is empty")
	}
	if !utf8.Valid(csv) {
		return nil, fmt.Errorf("CSV is not valid UTF-8")
	}
	// like SWAdmin (File.ReadAllLines): a line ends at CRLF, LF or a lone CR
	lines := strings.Split(strings.ReplaceAll(strings.ReplaceAll(string(csv), "\r\n", "\n"), "\r", "\n"), "\n")
	var rows []resRow
	for i, ln := range lines {
		ln = strings.TrimSuffix(ln, "\r")
		if i == 0 {
			continue // header
		}
		if ln == "" {
			continue
		}
		cells := strings.Split(ln, "\t")
		if len(cells) > len(fields) {
			for _, c := range cells[len(fields):] {
				if c != "" {
					return nil, fmt.Errorf("line %d has more values than the table has columns (%d)", i+1, len(fields))
				}
			}
			cells = cells[:len(fields)]
		}
		if len(cells) < len(fields) {
			return nil, fmt.Errorf("line %d: missing value for column %d (%s)", i+1, len(cells)+1, fields[len(cells)].Name)
		}
		var body bytes.Buffer
		var sum int64
		keyEnd := 0
		for j, f := range fields {
			v, err := putField(&body, f.Type, cells[j])
			if err != nil {
				return nil, fmt.Errorf("line %d, column %d (%s): %v", i+1, j+1, f.Name, err)
			}
			sum += v
			if j == 0 {
				keyEnd = body.Len()
			}
		}
		raw := body.Bytes()
		rows = append(rows, resRow{raw: raw, sum: sum, key: string(raw[:keyEnd])})
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("CSV has no data rows")
	}
	return rows, nil
}

// readRes splits a table made by the game into rows, using the schema. It fails if the bytes do not fit the
// schema exactly (the layout changed, or the schema belongs to another build).
func readRes(fields []resField, b []byte) ([]resRow, error) {
	le := binary.LittleEndian
	if len(fields) == 0 {
		return nil, fmt.Errorf("the schema has no columns for this table")
	}
	if len(b) < 4 {
		return nil, fmt.Errorf("table is too short")
	}
	n := int(le.Uint32(b))
	pos := 4
	need := func(k int) error {
		if k < 0 || pos+k > len(b) {
			return fmt.Errorf("table ends in the middle of a row")
		}
		return nil
	}
	rows := make([]resRow, 0, n)
	for r := 0; r < n; r++ {
		start := pos
		var sum int64
		keyEnd := 0
		for j, f := range fields {
			switch strings.ToLower(f.Type) {
			case "string":
				if err := need(2); err != nil {
					return nil, err
				}
				l := int(le.Uint16(b[pos:]))
				pos += 2
				if err := need(l * 2); err != nil {
					return nil, err
				}
				sum += int64(l)
				for k := 0; k < l; k++ {
					c := le.Uint16(b[pos+2*k:])
					sum += int64(c&0xFF) + int64(c>>8)
				}
				pos += l * 2
			case "byte", "bool", "boolean":
				if err := need(1); err != nil {
					return nil, err
				}
				sum += int64(b[pos])
				pos++
			case "uint16":
				if err := need(2); err != nil {
					return nil, err
				}
				sum += int64(le.Uint16(b[pos:]))
				pos += 2
			case "int16":
				if err := need(2); err != nil {
					return nil, err
				}
				sum += int64(int16(le.Uint16(b[pos:])))
				pos += 2
			case "uint32":
				if err := need(4); err != nil {
					return nil, err
				}
				sum += int64(le.Uint32(b[pos:]))
				pos += 4
			case "int32":
				if err := need(4); err != nil {
					return nil, err
				}
				sum += int64(int32(le.Uint32(b[pos:])))
				pos += 4
			case "single", "float":
				if err := need(4); err != nil {
					return nil, err
				}
				sum += floorSingle(math.Float32frombits(le.Uint32(b[pos:])))
				pos += 4
			default:
				return nil, fmt.Errorf("column type %q is not supported", f.Type)
			}
			if j == 0 {
				keyEnd = pos - start
			}
		}
		raw := b[start:pos]
		rows = append(rows, resRow{raw: raw, sum: sum, key: string(raw[:keyEnd])})
	}
	// footer: uint16 length + that many characters, and nothing after it
	if pos+2 > len(b) {
		return nil, fmt.Errorf("table has no footer")
	}
	fl := int(le.Uint16(b[pos:]))
	if pos+2+fl != len(b) {
		return nil, fmt.Errorf("table has %d unexpected byte(s) - the columns do not fit this table", len(b)-pos-2-fl)
	}
	return rows, nil
}

// mergeWithGame builds the table to write: the game's rows in the game's order, where a row's text comes
// from the translation when the translation has that row (same first column), and from the game itself
// when it does not (a row added by a game update, left in the game's language). Rows only the translation
// has (removed by the game) are left out, so the table always has exactly the game's rows.
type mergeStats struct{ Game, Translated, FromGame, Dropped int }

func mergeWithGame(tr, game []resRow) ([]resRow, mergeStats, error) {
	st := mergeStats{Game: len(game)}
	byKey := map[string][]resRow{}
	for _, r := range tr {
		byKey[r.key] = append(byKey[r.key], r)
	}
	used := map[string]bool{}
	var out []resRow
	for _, g := range game {
		if rs, ok := byKey[g.key]; ok {
			if used[g.key] {
				continue
			}
			used[g.key] = true
			out = append(out, rs...)
			st.Translated += len(rs)
			continue
		}
		out = append(out, g)
		st.FromGame++
	}
	for k, rs := range byKey {
		if !used[k] {
			st.Dropped += len(rs)
		}
	}
	// a translation that hardly matches the game is a different table (renumbered ids): do not mix them
	if len(game) >= 20 && st.Translated*2 < len(game) {
		return nil, st, fmt.Errorf("only %d of the game's %d rows are in the translation - the rows were probably renumbered", st.Translated, len(game))
	}
	return out, st, nil
}

// putField writes one field and returns what it adds to the table's checksum number.
func putField(w *bytes.Buffer, typ, s string) (int64, error) {
	le := binary.LittleEndian
	switch strings.ToLower(typ) {
	case "string":
		s = strings.ReplaceAll(s, "<br>", "\n")
		u := utf16.Encode([]rune(s))
		if len(u) > 0xFFFF {
			return 0, fmt.Errorf("text is too long (%d characters)", len(u))
		}
		binary.Write(w, le, uint16(len(u)))
		binary.Write(w, le, u)
		add := int64(len(u))
		for _, c := range u {
			add += int64(c&0xFF) + int64(c>>8)
		}
		return add, nil
	case "byte":
		v, err := strconv.ParseUint(s, 10, 8)
		if err != nil {
			return 0, numErr(s, "byte", err)
		}
		w.WriteByte(byte(v))
		return int64(v), nil
	case "bool", "boolean":
		switch strings.ToLower(s) {
		case "0", "false":
			w.WriteByte(0)
			return 0, nil
		case "1", "true":
			w.WriteByte(1)
			return 1, nil
		}
		return 0, fmt.Errorf("%q is not a yes/no value", s)
	case "uint16":
		v, err := strconv.ParseUint(s, 10, 16)
		if err != nil {
			return 0, numErr(s, "uint16", err)
		}
		binary.Write(w, le, uint16(v))
		return int64(v), nil
	case "int16":
		v, err := strconv.ParseInt(s, 10, 16)
		if err != nil {
			return 0, numErr(s, "int16", err)
		}
		binary.Write(w, le, int16(v))
		return v, nil
	case "uint32":
		v, err := strconv.ParseUint(s, 10, 32)
		if err != nil {
			return 0, numErr(s, "uint32", err)
		}
		binary.Write(w, le, uint32(v))
		return int64(v), nil
	case "int32":
		v, err := strconv.ParseInt(s, 10, 32)
		if err != nil {
			return 0, numErr(s, "int32", err)
		}
		binary.Write(w, le, int32(v))
		return v, nil
	case "single", "float":
		f, err := strconv.ParseFloat(s, 32)
		if err != nil {
			if s == "" {
				return 0, fmt.Errorf("missing value (single expected)")
			}
			return 0, fmt.Errorf("%q is not a valid single", s)
		}
		if math.IsInf(f, 0) || math.Abs(f) >= 7.9e28 {
			return 0, fmt.Errorf("%q is out of range", s)
		}
		binary.Write(w, le, math.Float32bits(float32(f)))
		return floorSingle(float32(f)), nil
	default:
		// double/64-bit fields do not occur in the string/script tables, and how SWAdmin adds them
		// to the checksum has not been verified - do not guess.
		return 0, fmt.Errorf("field type %q is not supported by the built-in converter (use Converter=swadmin)", typ)
	}
}

// floorSingle adds a float to the checksum the way SWAdmin does: Math.Floor(Convert.ToDecimal(float)).
// Converting a float to decimal rounds it to 7 significant digits first.
func floorSingle(f float32) int64 {
	r, ok := new(big.Rat).SetString(strconv.FormatFloat(float64(f), 'g', 7, 32))
	if !ok {
		return 0
	}
	return new(big.Int).Div(r.Num(), r.Denom()).Int64()
}

func numErr(s, typ string, err error) error {
	if s == "" {
		return fmt.Errorf("missing value (%s expected)", typ)
	}
	return fmt.Errorf("%q is not a valid %s", s, typ)
}

// convertCSVDir converts every CSV in csvDir whose table exists in `existing` (table file names as they
// are in the game) and writes the result to outDir under the game's own file name.
// It returns the names written, plus one message per problem or skipped file.
func convertCSVDir(schemaPath, csvDir, outDir string, existing []string) (done []string, problems []string, err error) {
	done, problems, _, err = convertCSVDirMerge(schemaPath, csvDir, outDir, existing, false)
	return
}

// convertCSVDirMerge is convertCSVDir; with merge set, each table is compared with the game's own table
// found in outDir (see mergeWithGame) and notes tells what was kept from the game.
func convertCSVDirMerge(schemaPath, csvDir, outDir string, existing []string, merge bool) (done []string, problems []string, notes []string, err error) {
	sch, err := loadResSchema(schemaPath)
	if err != nil {
		return nil, nil, nil, err
	}
	byLower := map[string]string{}
	for _, n := range existing {
		byLower[strings.ToLower(n)] = n
	}
	ents, err := os.ReadDir(csvDir)
	if err != nil {
		return nil, nil, nil, err
	}
	var names []string
	for _, e := range ents {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".csv") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, cn := range names {
		resName := strings.TrimSuffix(cn, filepath.Ext(cn)) + ".res"
		game, ok := byLower[strings.ToLower(resName)]
		if !ok {
			problems = append(problems, fmt.Sprintf("%s: not a table that is being patched - skipped", cn))
			continue
		}
		fields, ok := sch[strings.ToLower(resName)]
		if !ok {
			problems = append(problems, fmt.Sprintf("%s: error - no entry for %s in the schema", cn, resName))
			continue
		}
		b, err := os.ReadFile(filepath.Join(csvDir, cn))
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: error - %v", cn, err))
			continue
		}
		var out []byte
		if merge {
			trRows, err := encodeCSV(fields, b)
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: error - %v", cn, err))
				continue
			}
			gb, err := os.ReadFile(filepath.Join(outDir, game))
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: error - cannot read the game's table: %v", cn, err))
				continue
			}
			gameRows, err := readRes(fields, gb)
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: error - the game's table does not fit the table layout (%v) - table left as the game has it", cn, err))
				continue
			}
			merged, st, err := mergeWithGame(trRows, gameRows)
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: error - %v - table left as the game has it", cn, err))
				continue
			}
			if st.FromGame > 0 || st.Dropped > 0 {
				msg := fmt.Sprintf("%s: the game has %d row(s) the translation does not have yet - kept in the game's own language", strings.TrimSuffix(cn, filepath.Ext(cn)), st.FromGame)
				if st.Dropped > 0 {
					msg += fmt.Sprintf("; %d translated row(s) are no longer in the game and were left out", st.Dropped)
				}
				notes = append(notes, msg)
			}
			out = buildRes(merged)
		} else {
			out, err = csvToRes(fields, b)
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: error - %v", cn, err))
				continue
			}
		}
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			return done, problems, notes, err
		}
		if err := os.WriteFile(filepath.Join(outDir, game), out, 0o644); err != nil {
			return done, problems, notes, err
		}
		done = append(done, game)
	}
	return done, problems, notes, nil
}
