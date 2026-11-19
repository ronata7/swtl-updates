package rowpatch

import (
	"bytes"
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

func roundTrip(t *testing.T, old, new string) []byte {
	t.Helper()
	p := Make([]byte(old), []byte(new))
	got, err := Apply([]byte(old), p)
	if err != nil {
		t.Fatalf("apply: %v\nold=%q\nnew=%q\npatch=%q", err, old, new, p)
	}
	if string(got) != new {
		t.Fatalf("mismatch\nold=%q\nnew=%q\ngot=%q\npatch=%q", old, new, got, p)
	}
	return p
}

func TestSimpleCases(t *testing.T) {
	cases := [][2]string{
		{"", ""},
		{"a", "a"},
		{"a\n", "a\n"},
		{"h\n1\ta\n2\tb\n3\tc\n", "h\n1\ta\n2\tB\n3\tc\n"},         // change
		{"h\n1\ta\n2\tb\n3\tc\n", "h\n1\ta\n3\tc\n"},               // delete
		{"h\n1\ta\n3\tc\n", "h\n1\ta\n2\tb\n3\tc\n"},               // insert
		{"h\n1\ta\n2\tb\n3\tc\n", "h\n3\tc\n2\tb\n1\ta\n"},         // reorder
		{"h\n1\ta\n1\tb\n", "h\n1\ta\n1\tc\n1\td\n"},               // duplicate ids
		{"h\r\n1\ta\r\n2\tb\r\n", "h\r\n1\ta\r\n2\tb\r\n3\tc\r\n"}, // CRLF
		{"h\n1\ta\n", "h\n1\ta"},                                   // no final newline
		{"x", "h\n1\ta\n"},                                         // unrelated
		{"h\n1\ta\n2\tb\n", ""},                                    // all gone
		{"h\n1\t\n2\t+plus\n3\t=eq\n4\t-minus\n", "h\n1\t\n2\t+plus\n3\t=eq\n4\t-minus\n5\t+\n"},
	}
	for _, c := range cases {
		roundTrip(t, c[0], c[1])
	}
}

func TestRandomEdits(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	for iter := 0; iter < 200; iter++ {
		var rows []string
		for i := 0; i < 300; i++ {
			rows = append(rows, fmt.Sprintf("%d\ttext %d <br> more", i*3+r.Intn(2), r.Intn(1000)))
		}
		old := "h\r\n" + strings.Join(rows, "\r\n") + "\r\n"
		nr := append([]string(nil), rows...)
		for k := 0; k < 1+r.Intn(15); k++ {
			switch r.Intn(4) {
			case 0:
				i := r.Intn(len(nr))
				nr[i] = nr[i] + " edited"
			case 1:
				i := r.Intn(len(nr))
				nr = append(nr[:i], nr[i+1:]...)
			case 2:
				i := r.Intn(len(nr))
				nr = append(nr[:i], append([]string{fmt.Sprintf("%d\tnew row", 100000+k)}, nr[i:]...)...)
			case 3:
				i, j := r.Intn(len(nr)), r.Intn(len(nr))
				nr[i], nr[j] = nr[j], nr[i]
			}
		}
		roundTrip(t, old, "h\r\n"+strings.Join(nr, "\r\n")+"\r\n")
	}
}

func TestSmallPatchForSmallChange(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("h\n")
	for i := 0; i < 50000; i++ {
		fmt.Fprintf(&sb, "%d\tThis is some translated line number %d with <br> a break\n", i, i)
	}
	old := sb.String()
	new := strings.Replace(old, "line number 25000 ", "line number 25000 (edited) ", 1)
	p := roundTrip(t, old, new)
	if len(p) > 400 {
		t.Fatalf("patch for one changed row is %d bytes", len(p))
	}
}

func TestBadPatches(t *testing.T) {
	old := []byte("h\n1\ta\n")
	for _, p := range []string{"", "nonsense", magic + "\n=99\n", magic + "\n-99\n", magic + "\n=x\n", magic + "\n?1\n", magic + "\n=1", magic + "\n\n"} {
		if _, err := Apply(old, []byte(p)); err == nil {
			t.Errorf("patch %q should be refused", p)
		}
	}
}

func TestGzipLimit(t *testing.T) {
	z := Gzip(bytes.Repeat([]byte("a"), 100000))
	if _, err := Gunzip(z, 1000); err == nil {
		t.Fatal("should refuse to expand beyond the limit")
	}
	if b, err := Gunzip(z, 200000); err != nil || len(b) != 100000 {
		t.Fatalf("gunzip: %v", err)
	}
}
