package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Built-in replacement for "SWAdmin.Cli.exe -dumper_to_schema" (SWAdmin tools, see credits).
// The structs dumper writes one C# class per table (tb_Name.cs) with properties in file order:
//     public uint Unk0 { get; set; }
// The schema entry for it is  { "file": "tb_Name.res", "fields": [ { "name": "Unkown1", "type": "uint32" }, ... ] }
// (columns are numbered from 1). Verified against a real dump of 356 tables.

var (
	reClass = regexp.MustCompile(`class\s+(\w+)`)
	reProp  = regexp.MustCompile(`public\s+(\w+)\s+\w+\s*\{\s*get;\s*set;\s*\}`)
)

var csTypes = map[string]string{
	"byte": "byte", "sbyte": "byte", "bool": "boolean", "short": "int16", "ushort": "uint16",
	"int": "int32", "uint": "uint32", "long": "int64", "ulong": "uint64",
	"float": "single", "double": "double", "string": "string",
}

func parseDumperClass(src []byte) (resSchemaEntry, error) {
	src = bytes.TrimPrefix(src, []byte{0xEF, 0xBB, 0xBF})
	m := reClass.FindSubmatch(src)
	if m == nil {
		return resSchemaEntry{}, fmt.Errorf("no class found")
	}
	e := resSchemaEntry{File: string(m[1]) + ".res"}
	for i, p := range reProp.FindAllSubmatch(src, -1) {
		t, ok := csTypes[string(p[1])]
		if !ok {
			return e, fmt.Errorf("unknown field type %q", p[1])
		}
		e.Fields = append(e.Fields, resField{Name: fmt.Sprintf("Unkown%d", i+1), Type: t})
	}
	if len(e.Fields) == 0 {
		return e, fmt.Errorf("class %s has no fields", string(m[1]))
	}
	return e, nil
}

// dumperToSchema reads every .cs file in the dumper folders (later folders override earlier ones, like
// SWAdmin's -override) and returns the schema entries sorted by file name.
func dumperToSchema(dirs ...string) ([]resSchemaEntry, error) {
	byName := map[string]resSchemaEntry{}
	for _, d := range dirs {
		files, err := filepath.Glob(filepath.Join(d, "*.cs"))
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			b, err := os.ReadFile(f)
			if err != nil {
				return nil, err
			}
			e, err := parseDumperClass(b)
			if err != nil {
				return nil, fmt.Errorf("%s: %v", filepath.Base(f), err)
			}
			byName[strings.ToLower(e.File)] = e
		}
	}
	if len(byName) == 0 {
		return nil, fmt.Errorf("no .cs files found")
	}
	out := make([]resSchemaEntry, 0, len(byName))
	for _, e := range byName {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].File) < strings.ToLower(out[j].File) })
	return out, nil
}

func writeSchema(path string, es []resSchemaEntry) error {
	var b strings.Builder
	b.WriteString("[\r\n")
	for i, e := range es {
		fmt.Fprintf(&b, "  {\r\n    \"file\": %q,\r\n    \"fields\": [\r\n", e.File)
		for j, f := range e.Fields {
			sep := ","
			if j == len(e.Fields)-1 {
				sep = ""
			}
			fmt.Fprintf(&b, "      { \"name\": %q, \"type\": %q }%s\r\n", f.Name, f.Type, sep)
		}
		b.WriteString("    ]\r\n  }")
		if i < len(es)-1 {
			b.WriteString(",")
		}
		b.WriteString("\r\n")
	}
	b.WriteString("]\r\n")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// sameSchema compares two schema files by content (order of entries does not matter).
func sameSchema(a, b string) (bool, string) {
	ma, err := loadResSchema(a)
	if err != nil {
		return false, err.Error()
	}
	mb, err := loadResSchema(b)
	if err != nil {
		return false, err.Error()
	}
	if len(ma) != len(mb) {
		return false, fmt.Sprintf("%d tables vs %d tables", len(ma), len(mb))
	}
	for k, fa := range ma {
		fb, ok := mb[k]
		if !ok {
			return false, k + " is missing in the second schema"
		}
		if len(fa) != len(fb) {
			return false, fmt.Sprintf("%s has %d vs %d columns", k, len(fa), len(fb))
		}
		for i := range fa {
			if fa[i] != fb[i] {
				return false, fmt.Sprintf("%s column %d differs (%v vs %v)", k, i+1, fa[i], fb[i])
			}
		}
	}
	return true, ""
}
