// Package resfmt checks the FORMAT of a translation CSV against a table schema (column count and the
// type of every column), using the same rules the updater applies when it builds the game table.
// Nothing here looks at what the text says.
package resfmt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

type Field struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type entry struct {
	File   string  `json:"file"`
	Fields []Field `json:"fields"`
}

// LoadSchema reads schema.json keyed by lower-case table file name (for example "tb_shop_string.res").
func LoadSchema(path string) (map[string][]Field, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var es []entry
	if err := json.Unmarshal(bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF}), &es); err != nil {
		return nil, fmt.Errorf("schema file is not valid: %v", err)
	}
	m := map[string][]Field{}
	for _, e := range es {
		m[strings.ToLower(e.File)] = e.Fields
	}
	return m, nil
}

// ValidateCSV returns every format problem in the CSV (at most max; 0 = all), as "line N, column M (name): reason".
func ValidateCSV(fields []Field, csv []byte, max int) []string {
	var out []string
	add := func(s string) bool {
		out = append(out, s)
		return max > 0 && len(out) >= max
	}
	csv = bytes.TrimPrefix(csv, []byte{0xEF, 0xBB, 0xBF})
	if len(bytes.TrimSpace(csv)) == 0 {
		return []string{"file is empty"}
	}
	if !utf8.Valid(csv) {
		return []string{"file is not valid UTF-8 (save it as UTF-8)"}
	}
	rows := 0
	for i, ln := range strings.Split(string(csv), "\n") {
		ln = strings.TrimSuffix(ln, "\r")
		if i == 0 || ln == "" {
			continue
		}
		rows++
		cells := strings.Split(ln, "\t")
		if len(cells) > len(fields) {
			extra := false
			for _, c := range cells[len(fields):] {
				if c != "" {
					extra = true
				}
			}
			if extra {
				if add(fmt.Sprintf("line %d: %d columns, the table has %d (a tab inside a text?)", i+1, len(cells), len(fields))) {
					return out
				}
				continue
			}
			cells = cells[:len(fields)]
		}
		if len(cells) < len(fields) {
			if add(fmt.Sprintf("line %d: %d columns, the table has %d (missing value for column %d, %s)", i+1, len(cells), len(fields), len(cells)+1, fields[len(cells)].Name)) {
				return out
			}
			continue
		}
		for j, f := range fields {
			if msg := checkCell(f.Type, cells[j]); msg != "" {
				if add(fmt.Sprintf("line %d, column %d (%s): %s", i+1, j+1, f.Name, msg)) {
					return out
				}
			}
		}
	}
	if rows == 0 {
		out = append(out, "no data rows")
	}
	return out
}

func checkCell(typ, s string) string {
	num := func(bits int, signed bool) string {
		var err error
		if signed {
			_, err = strconv.ParseInt(s, 10, bits)
		} else {
			_, err = strconv.ParseUint(s, 10, bits)
		}
		if err != nil {
			if s == "" {
				return fmt.Sprintf("empty (a %s number is expected)", typ)
			}
			return fmt.Sprintf("%q is not a valid %s number", clip(s), typ)
		}
		return ""
	}
	switch strings.ToLower(typ) {
	case "string":
		if n := len(utf16.Encode([]rune(strings.ReplaceAll(s, "<br>", "\n")))); n > 0xFFFF {
			return fmt.Sprintf("text is too long (%d characters, limit 65535)", n)
		}
		return ""
	case "byte":
		return num(8, false)
	case "bool", "boolean":
		switch strings.ToLower(s) {
		case "0", "1", "true", "false":
			return ""
		}
		return fmt.Sprintf("%q is not a yes/no value (0 or 1)", clip(s))
	case "uint16":
		return num(16, false)
	case "int16":
		return num(16, true)
	case "uint32":
		return num(32, false)
	case "int32":
		return num(32, true)
	case "int64":
		return num(64, true)
	case "uint64":
		return num(64, false)
	case "single", "float", "double":
		if _, err := strconv.ParseFloat(s, 64); err != nil {
			return fmt.Sprintf("%q is not a valid decimal number", clip(s))
		}
		return ""
	}
	return fmt.Sprintf("unknown column type %q", typ)
}

func clip(s string) string {
	r := []rune(s)
	if len(r) > 30 {
		return string(r[:30]) + "..."
	}
	return s
}
