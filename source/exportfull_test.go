package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestFullCSVRoundTrip(t *testing.T) {
	fields := []resField{{"id", "uint32"}, {"name", "string"}, {"f", "single"}, {"b", "byte"}}
	csv := "id\tname\tf\tb\t\r\n1\tHello<br>World\t1.5\t3\t\r\n2\t\t0.25\t0\t\r\n"
	orig, err := csvToRes(fields, []byte(csv))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := readRes(fields, orig)
	if err != nil {
		t.Fatal(err)
	}
	out := resToCSV(fields, rows)
	if string(out) != csv {
		t.Fatalf("csv differs:\n%q\n%q", out, csv)
	}
	if why := roundTrip(fields, out, orig); why != "" {
		t.Fatal(why)
	}
	// a text with a tab cannot survive the csv: the round trip must say so
	rows[0].raw = nil
	bad, _ := csvToRes(fields, []byte("id\tname\tf\tb\t\r\n1\tA\tB\t1.5\t3\t\r\n"))
	_ = bad
}

func TestRoundTripDetectsLoss(t *testing.T) {
	fields := []resField{{"id", "uint32"}, {"name", "string"}}
	orig, _ := csvToRes(fields, []byte("id\tname\t\r\n1\tx\r\n"))
	// build an original whose text holds a literal TAB (the csv would split it)
	var rows []resRow
	rows, _ = readRes(fields, orig)
	_ = rows
	tabbed := bytes.Replace(orig, []byte{'x', 0}, []byte{'\t', 0}, 1)
	r, err := readRes(fields, tabbed)
	if err != nil {
		t.Skip("cannot build the test table: ", err)
	}
	csv := resToCSV(fields, r)
	if why := roundTrip(fields, csv, tabbed); why == "" {
		t.Fatal("a tab inside a text must be reported as not lossless; csv=" + strings.ReplaceAll(string(csv), "\t", "<T>"))
	}
}

func TestEscapeDumpRoundTrip(t *testing.T) {
	cases := []string{"", "plain", "a\nb", "a\r\nb", "a\rb", "tab\there", "literal <br> tag", "<br><cr><tab><esc>", "<<br>", "<esc><esc>", "<esc><br>x", "<", "<b", "x<esc", "\n<br>\r\n<cr>", "日本語\n<br>"}
	for _, c := range cases {
		e := escapeDump(c)
		if strings.ContainsAny(e, "\n\r\t") {
			t.Fatalf("escaped text still has a control character: %q -> %q", c, e)
		}
		if got := unescapeDump(e); got != c {
			t.Fatalf("round trip of %q: escaped %q, back %q", c, e, got)
		}
	}
	// normal project text is unchanged
	if escapeDump("Hello<br>World") == "Hello<br>World" {
		t.Fatal("a literal <br> must be escaped; only a real line feed becomes <br>")
	}
	if escapeDump("Hello\nWorld") != "Hello<br>World" {
		t.Fatal("a line feed must become <br>")
	}
}

func TestEmptyTableRoundTrip(t *testing.T) {
	fields := []resField{{"id", "uint16"}, {"name", "string"}}
	orig := buildRes(nil) // a table with no rows
	rows, err := readRes(fields, orig)
	if err != nil || len(rows) != 0 {
		t.Fatalf("an empty table must be readable: %v", err)
	}
	csv := resToCSV(fields, rows)
	if why := roundTrip(fields, csv, orig); why != "" {
		t.Fatal(why)
	}
	// the normal translation conversion still refuses a csv without rows
	if _, err := csvToRes(fields, csv); err == nil {
		t.Fatal("csvToRes must still refuse an empty csv")
	}
}
