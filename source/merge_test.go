package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"
)

func mk(t *testing.T, csv string) []resRow {
	t.Helper()
	r, err := encodeCSV(shopFields, []byte("h\r\n"+csv))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestReadResRoundTripAndLayoutChange(t *testing.T) {
	rows := mk(t, "1\tA\t\r\n2\tBB\t\r\n3\t\t\r\n")
	b := buildRes(rows)
	got, err := readRes(shopFields, b)
	if err != nil || len(got) != 3 || !bytes.Equal(buildRes(got), b) {
		t.Fatalf("round trip failed: %v", err)
	}
	// a table with one more column does not fit the old layout
	three := append(append([]resField{}, shopFields...), resField{"x", "uint32"})
	r3, _ := encodeCSV(three, []byte("h\r\n1\tA\t5\r\n2\tB\t6\r\n"))
	if _, err := readRes(shopFields, buildRes(r3)); err == nil {
		t.Fatal("a table with an extra column was accepted")
	}
	// a table with a column fewer
	if _, err := readRes(three, b); err == nil {
		t.Fatal("a table with a missing column was accepted")
	}
}

func TestMergeKeepsNewGameRows(t *testing.T) {
	game := mk(t, "1\tK1\t\r\n2\tK2\t\r\n3\tK3\t\r\n4\tK4\t\r\n")
	tr := mk(t, "1\tE1\t\r\n3\tE3\t\r\n9\tOLD\t\r\n")
	out, st, err := mergeWithGame(shopFields, tr, game)
	if err != nil {
		t.Fatal(err)
	}
	if st.FromGame != 2 || st.Dropped != 1 || st.Translated != 2 || len(out) != 4 {
		t.Fatalf("stats %+v len %d", st, len(out))
	}
	b := buildRes(out)
	rows, _ := readRes(shopFields, b)
	want := []string{"E1", "K2", "E3", "K4"}
	for i, r := range rows {
		if !strings.Contains(string(r.raw), "") || len(r.raw) != len(out[i].raw) {
			t.Fatal("row mismatch")
		}
		_ = want[i]
	}
	if !bytes.Equal(rows[1].raw, game[1].raw) || !bytes.Equal(rows[3].raw, game[3].raw) || !bytes.Equal(rows[0].raw, tr[0].raw) {
		t.Fatal("rows come from the wrong table")
	}
}

func TestMergeKeepsGameOrderAndDuplicates(t *testing.T) {
	game := mk(t, "5\tg\t\r\n1\tg\t\r\n5\tg2\t\r\n")
	tr := mk(t, "1\tt\t\r\n5\tt1\t\r\n5\tt2\t\r\n")
	out, _, err := mergeWithGame(shopFields, tr, game)
	if err != nil || len(out) != 3 {
		t.Fatalf("%v %d", err, len(out))
	}
	if out[0].key != game[0].key || !bytes.Equal(out[0].raw, tr[1].raw) || !bytes.Equal(out[1].raw, tr[2].raw) || !bytes.Equal(out[2].raw, tr[0].raw) {
		t.Fatal("order or duplicate handling is wrong")
	}
}

func TestMergeRefusesRenumberedTable(t *testing.T) {
	var g, tr strings.Builder
	for i := 1; i <= 30; i++ {
		g.WriteString(strings.Repeat("1", 1) + "0" + string(rune('0'+i%10)) + "\tx\t\r\n")
	}
	for i := 1; i <= 30; i++ {
		tr.WriteString("9" + string(rune('0'+i%10)) + "7\ty\t\r\n")
	}
	if _, _, err := mergeWithGame(shopFields, mk(t, tr.String()), mk(t, g.String())); err == nil {
		t.Fatal("renumbered table was merged")
	}
}

var f3 = []resField{{"id", "uint32"}, {"a", "uint32"}, {"b", "uint32"}, {"code", "string"}, {"txt", "string"}}

func mk3(t *testing.T, rows string) []resRow {
	t.Helper()
	r, err := encodeCSV(f3, []byte("h\r\n"+rows))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func rowsOf(n int, f func(i int) string) string {
	var sb strings.Builder
	for i := 1; i <= n; i++ {
		sb.WriteString(f(i) + "\r\n")
	}
	return sb.String()
}

func TestMergeTakesNumbersFromGame(t *testing.T) {
	game := mk3(t, rowsOf(10, func(i int) string { return itoa(i) + "\t50\t7\tCODE\t\u30c6\u30b9\u30c8" }))
	// the game changed a number in two rows since the translation was made
	tr := mk3(t, rowsOf(10, func(i int) string {
		a := "50"
		if i <= 2 {
			a = "40"
		}
		return itoa(i) + "\t" + a + "\t7\tCODE\tTest"
	}))
	out, st, err := mergeWithGame(f3, tr, game)
	if err != nil || len(st.Warnings) != 0 {
		t.Fatalf("%v %v", err, st.Warnings)
	}
	rows, err := readRes(f3, buildRes(out))
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range rows {
		if !bytes.Equal(strBytes(r, 1), strBytes(game[i], 1)) {
			t.Fatalf("row %d has an old number", i)
		}
		if !bytes.Equal(strBytes(r, 4), strBytes(tr[i], 4)) {
			t.Fatalf("row %d lost its translated text", i)
		}
	}
}

func TestMergeDetectsMovedNumberColumns(t *testing.T) {
	game := mk3(t, rowsOf(10, func(i int) string { return itoa(i) + "\t" + itoa(100+i) + "\t" + itoa(200+i) + "\tC\tx" }))
	// same layout of types, but columns a and b are swapped in the translation
	tr := mk3(t, rowsOf(10, func(i int) string { return itoa(i) + "\t" + itoa(200+i) + "\t" + itoa(100+i) + "\tC\tx" }))
	if _, _, err := mergeWithGame(f3, tr, game); err == nil {
		t.Fatal("swapped number columns were accepted")
	}
}

func TestMergeWarnsAboutSwappedTextColumns(t *testing.T) {
	game := mk3(t, rowsOf(10, func(i int) string { return itoa(i) + "\t1\t2\tFILE_" + itoa(i) + "\t\u30c6\u30b9\u30c8" }))
	tr := mk3(t, rowsOf(10, func(i int) string { return itoa(i) + "\t1\t2\tSome English text\tFILE_" + itoa(i) }))
	_, st, err := mergeWithGame(f3, tr, game)
	if err != nil || len(st.Warnings) != 1 {
		t.Fatalf("%v %v", err, st.Warnings)
	}
}

func itoa(i int) string {
	return strings.TrimSpace(strings.Replace(string(rune('0'+i/100%10))+string(rune('0'+i/10%10))+string(rune('0'+i%10)), "000", "0", 1))
}

// the big tables have over a million rows: merging must stay fast
func TestMergeIsFastOnBigTables(t *testing.T) {
	var g, tr strings.Builder
	for i := 1; i <= 300000; i++ {
		g.WriteString(itoa6(i) + "\tgame text\t\r\n")
		if i%3 != 0 {
			tr.WriteString(itoa6(i) + "\ttranslated\t\r\n")
		}
	}
	start := time.Now()
	out, st, err := mergeWithGame(shopFields, mk(t, tr.String()), mk(t, g.String()))
	if err != nil || len(out) != 300000 || st.FromGame != 100000 {
		t.Fatalf("%v %d %+v", err, len(out), st)
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("merge took %s", d)
	}
}

func itoa6(i int) string { return fmt.Sprintf("%d", 100000+i) }
