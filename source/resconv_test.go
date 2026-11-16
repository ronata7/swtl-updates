package main

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	if _, err := csvToRes([]resField{{"a", "single"}}, []byte("h\r\n1.5\r\n")); err == nil {
		t.Fatal("float fields must be refused, not guessed")
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
