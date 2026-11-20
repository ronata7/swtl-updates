package archivekey

import (
	"strings"
	"testing"
)

func TestRoundTripAndFind(t *testing.T) { // made-up passwords only
	l12, err := Encode("data12", "fake-password-12")
	if err != nil {
		t.Fatal(err)
	}
	l11, _ := Encode("data11", "fake-password-11")
	newer, _ := Encode("DATA12", "fake-password-12-new")
	if strings.Contains(l12, "fake-password") {
		t.Fatal("password visible in the line")
	}
	if n, pw, err := Decode(l12); err != nil || n != "data12" || pw != "fake-password-12" {
		t.Fatalf("%v %q %q", err, n, pw)
	}
	text := "Some readme text\r\n" + l12 + "\r\n" + l11 + "\r\nmore text  " + newer + "\r\n"
	if pw, ok := Find(text, "data12"); !ok || pw != "fake-password-12-new" {
		t.Fatalf("newest line must win: %q %v", pw, ok)
	}
	if pw, ok := Find(text, "data11"); !ok || pw != "fake-password-11" {
		t.Fatalf("%q %v", pw, ok)
	}
	if _, ok := Find(text, "data13"); ok {
		t.Fatal("found a key that is not there")
	}
}

func TestDamagedLines(t *testing.T) {
	l, _ := Encode("data12", "x-y-z")
	for _, bad := range []string{"", "hello", "SWTLKEY1:data12", "SWTLKEY1:data12:@@@", l[:len(l)-6], strings.Replace(l, "data12", "data11", 1)} {
		if _, _, err := Decode(bad); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
	if _, ok := Find(l[:len(l)-6], "data12"); ok {
		t.Fatal("a cut line was accepted")
	}
}
