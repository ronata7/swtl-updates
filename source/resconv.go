package main

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
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

// csvToRes converts the text of one translation CSV into the binary table.
func csvToRes(fields []resField, csv []byte) ([]byte, error) {
	csv = bytes.TrimPrefix(csv, []byte{0xEF, 0xBB, 0xBF})
	if len(bytes.TrimSpace(csv)) == 0 {
		return nil, fmt.Errorf("CSV file is empty")
	}
	if !utf8.Valid(csv) {
		return nil, fmt.Errorf("CSV is not valid UTF-8")
	}
	lines := strings.Split(string(csv), "\n")
	var body bytes.Buffer
	var sum int64
	rows := 0
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
		for j, f := range fields {
			v, err := putField(&body, f.Type, cells[j])
			if err != nil {
				return nil, fmt.Errorf("line %d, column %d (%s): %v", i+1, j+1, f.Name, err)
			}
			sum += v
		}
		rows++
	}
	if rows == 0 {
		return nil, fmt.Errorf("CSV has no data rows")
	}
	var out bytes.Buffer
	binary.Write(&out, binary.LittleEndian, uint32(rows))
	out.Write(body.Bytes())
	h := md5.Sum([]byte(strconv.FormatInt(sum, 10)))
	hx := hex.EncodeToString(h[:])
	binary.Write(&out, binary.LittleEndian, uint16(len(hx)))
	out.WriteString(hx)
	return out.Bytes(), nil
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
	default:
		// single/double/64-bit fields do not occur in the string/script tables, and how SWAdmin adds them
		// to the checksum has not been verified - do not guess.
		return 0, fmt.Errorf("field type %q is not supported by the built-in converter (use Converter=swadmin)", typ)
	}
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
	sch, err := loadResSchema(schemaPath)
	if err != nil {
		return nil, nil, err
	}
	byLower := map[string]string{}
	for _, n := range existing {
		byLower[strings.ToLower(n)] = n
	}
	ents, err := os.ReadDir(csvDir)
	if err != nil {
		return nil, nil, err
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
		out, err := csvToRes(fields, b)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: error - %v", cn, err))
			continue
		}
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			return done, problems, err
		}
		if err := os.WriteFile(filepath.Join(outDir, game), out, 0o644); err != nil {
			return done, problems, err
		}
		done = append(done, game)
	}
	return done, problems, nil
}
