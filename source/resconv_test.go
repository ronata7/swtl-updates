package main

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"swtlupdater/internal/resfmt"
	"testing"
)

var shopFields = []resField{{"Unkown1", "uint32"}, {"Unkown2", "string"}}

func TestCsvToResKnownChecksums(t *testing.T) {
	// checksums produced by SWAdmin for these made-up tables
	cases := []struct{ rows, want string }{
		{"1\tA\t\r\n", "735b90b45681 25ed6c3f678819b6e058"},
		{"1\tB\t\r\n", "a3f390d88e4c41f2747bfa2f1b5f87db"},
		{"2\tA\t\r\n", "a3f390d88e4c41f2747bfa2f1b5f87db"},
		{"1\tA\t\r\n2\tB\t\r\n", "42a0e188f5033bc65bf8d78622277c4e"},
		{"2\tB\t\r\n1\tA\t\r\n", "42a0e188f5033bc65bf8d78622277c4e"},
		{"1\t가\t\r\n", "bf8229696f7a3bb4700cfddef19fa23f"},
		{"1\t\t\r\n", "c4ca4238a0b923820dcc509a6f75849b"},
	}
	for _, c := range cases {
		want := c.want
		if want == "735b90b45681 25ed6c3f678819b6e058" {
			want = "735b90b4568125ed6c3f678819b6e058"
		}
		out, err := csvToRes(shopFields, []byte("Unkown1\tUnkown2\t\r\n"+c.rows))
		if err != nil {
			t.Fatal(err)
		}
		got := string(out[len(out)-32:])
		if got != want {
			t.Errorf("%q: checksum %s want %s", c.rows, got, want)
		}
		if out[len(out)-34] != 32 || out[len(out)-33] != 0 {
			t.Errorf("footer length prefix wrong")
		}
	}
}

func TestCsvToResExactBytes(t *testing.T) {
	out, err := csvToRes(shopFields, []byte("Unkown1\tUnkown2\t\r\n1\tA\t\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	sum := 1 + 1 + 65
	h := md5.Sum([]byte(strconv.Itoa(sum)))
	want := "\x01\x00\x00\x00" + "\x01\x00\x00\x00" + "\x01\x00" + "A\x00" + "\x20\x00" + hex.EncodeToString(h[:])
	if string(out) != want {
		t.Fatalf("got % x", out)
	}
}

func TestCsvToResLineBreakAndErrors(t *testing.T) {
	out, err := csvToRes(shopFields, []byte("h\r\n1\ta<br>b\t\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if out[8] != 3 || out[12] != 10 { // 3 characters, the middle one is a real line feed
		t.Fatalf("line break not converted: % x", out)
	}
	for _, bad := range []string{"", "h\r\n", "h\r\nx\tA\t\r\n", "h\r\n1\r\n", "h\r\n1\tA\tZ\r\n", "h\r\n-1\tA\t\r\n"} {
		if _, err := csvToRes(shopFields, []byte(bad)); err == nil {
			t.Errorf("expected an error for %q", bad)
		}
	}
}

// Byte-for-byte comparison with tables written by SWAdmin from the same made-up CSVs.
func TestCsvToResMatchesSWAdmin(t *testing.T) {
	files, _ := filepath.Glob("testdata/swadmin/*.csv")
	if len(files) < 9 {
		t.Fatalf("fixtures missing (%d)", len(files))
	}
	for _, f := range files {
		csv, _ := os.ReadFile(f)
		want, err := os.ReadFile(strings.TrimSuffix(f, ".csv") + ".res")
		if err != nil {
			t.Fatal(err)
		}
		got, err := csvToRes(shopFields, csv)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s differs from SWAdmin's file", filepath.Base(f))
		}
	}
}

func TestChecksumUsesFieldValues(t *testing.T) {
	// number fields add their value (not their bytes); a string adds its length plus the sum of its UTF-16 bytes
	f := []resField{{"a", "uint32"}, {"b", "string"}, {"c", "uint16"}, {"d", "byte"}}
	out, err := csvToRes(f, []byte("h\r\n100001\tAB\t300\t7\t\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	sum := 100001 + (2 + 65 + 66) + 300 + 7
	h := md5.Sum([]byte(strconv.Itoa(sum)))
	if got := string(out[len(out)-32:]); got != hex.EncodeToString(h[:]) {
		t.Fatalf("checksum %s, want md5 of %d", got, sum)
	}
	if _, err := csvToRes([]resField{{"a", "double"}}, []byte("h\r\n1.5\r\n")); err == nil {
		t.Fatal("double fields must be refused, not guessed")
	}
}

// A real table: the game's tb_Shop_String as written by SWAdmin, and the CSV it was made from.
func TestCsvToResRealShopTable(t *testing.T) {
	csv, err := os.ReadFile("testdata/real/tb_Shop_String.csv")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := os.ReadFile("testdata/real/tb_Shop_String.res")
	got, err := csvToRes(shopFields, csv)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("differs from SWAdmin's table (%d vs %d bytes)", len(got), len(want))
	}
}

// The standalone checker (internal/resfmt) must accept and reject exactly what the converter does.
func TestResfmtAgreesWithConverter(t *testing.T) {
	fields := []resField{{"a", "uint32"}, {"b", "string"}, {"c", "uint16"}, {"d", "byte"}, {"e", "int16"}, {"f", "bool"}}
	rf := make([]resfmt.Field, len(fields))
	for i, f := range fields {
		rf[i] = resfmt.Field{Name: f.Name, Type: f.Type}
	}
	cases := []string{
		"h\r\n1\ttext\t2\t3\t-4\t1\r\n",
		"h\r\n1\ttext\t2\t3\t-4\r\n",           // missing column
		"h\r\n1\ttext\t2\t3\t-4\t1\textra\r\n", // extra column
		"h\r\n-1\ttext\t2\t3\t-4\t1\r\n",       // negative uint
		"h\r\n1\ttext\t70000\t3\t-4\t1\r\n",    // uint16 overflow
		"h\r\n1\ttext\t2\t300\t-4\t1\r\n",      // byte overflow
		"h\r\n1\ttext\t2\t3\t40000\t1\r\n",     // int16 overflow
		"h\r\n1\ttext\t2\t3\t-4\tyes\r\n",      // bad bool
		"h\r\n1\ttext\t\t3\t-4\t1\r\n",         // empty number
		"h\r\n1\ttext\t2\t3\t-4\t1\t\r\n",      // trailing empty cell is fine
		"h\r\n",                                // no rows
		"\xff\xfe",                             // not UTF-8
	}
	for _, c := range cases {
		_, err := csvToRes(fields, []byte(c))
		probs := resfmt.ValidateCSV(rf, []byte(c), 0)
		if (err == nil) != (len(probs) == 0) {
			t.Errorf("disagree on %q: converter err=%v, checker=%v", c, err, probs)
		}
	}
}

func TestSingleAndLoneCR(t *testing.T) {
	f := []resField{{"a", "uint32"}, {"b", "single"}}
	out, err := csvToRes(f, []byte("h\r\n1\t1.5\r\n2\t-1.5\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	// 2 rows; row1: 01000000 0000C03F ; row2: 02000000 000 0C0BF
	want := []byte{2, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0xC0, 0x3F, 2, 0, 0, 0, 0, 0, 0xC0, 0xBF}
	if !bytes.HasPrefix(out, want) {
		t.Fatalf("bytes % x", out[:20])
	}
	// checksum number = 1+floor(1.5) + 2+floor(-1.5) = 1+1+2-2 = 2
	h := md5.Sum([]byte("2"))
	if string(out[len(out)-32:]) != hex.EncodeToString(h[:]) {
		t.Fatalf("checksum wrong: %s", out[len(out)-32:])
	}
	// a lone CR ends a line, like SWAdmin
	o1, e1 := csvToRes(shopFields, []byte("h\r\n1\ta\r2\tb\r\n"))
	o2, e2 := csvToRes(shopFields, []byte("h\r\n1\ta\r\n2\tb\r\n"))
	if e1 != nil || e2 != nil || !bytes.Equal(o1, o2) {
		t.Fatalf("lone CR not handled: %v %v", e1, e2)
	}
}

func TestFloorSingleExact(t *testing.T) {
	if got := floorSingle(2.2052612e+07); got != 22052612 {
		t.Fatalf("got %d", got)
	}
	if got := floorSingle(-1.5); got != -1 {
		t.Fatalf("got %d", got)
	}
	if got := floorSingle(0.45); got != 0 {
		t.Fatalf("got %d", got)
	}
}
