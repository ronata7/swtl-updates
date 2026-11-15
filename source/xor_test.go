package main

import (
	"bytes"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

func TestXorFile(t *testing.T) {
	d := t.TempDir()
	// bigger than the 1 MiB buffer and not a multiple of it
	data := make([]byte, (3<<20)+12345)
	rand.New(rand.NewSource(1)).Read(data)
	in, mid, back := filepath.Join(d, "a"), filepath.Join(d, "b"), filepath.Join(d, "c")
	if err := os.WriteFile(in, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := xorFile(in, mid, dataKey); err != nil {
		t.Fatal(err)
	}
	m, _ := os.ReadFile(mid)
	if len(m) != len(data) {
		t.Fatalf("size %d != %d", len(m), len(data))
	}
	for _, i := range []int{0, 1, 4095, len(data) - 1, 1 << 20, (1 << 20) + 1} {
		if m[i] != data[i]^0x55 {
			t.Fatalf("byte %d wrong", i)
		}
	}
	if err := xorFile(mid, back, dataKey); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(back)
	if !bytes.Equal(b, data) {
		t.Fatal("round trip differs")
	}
	if _, err := os.Stat(mid + ".xortmp"); err == nil {
		t.Fatal("temp file left behind")
	}
}

func TestXorHeader(t *testing.T) {
	d := t.TempDir()
	zip, enc := filepath.Join(d, "z"), filepath.Join(d, "z.v")
	os.WriteFile(zip, []byte("PK\x03\x04rest"), 0o644)
	if err := xorFile(zip, enc, dataKey); err != nil {
		t.Fatal(err)
	}
	if !isXoredZip(enc) {
		t.Fatal("header check failed on a file we just encoded")
	}
}

func TestXorEmptyFails(t *testing.T) {
	d := t.TempDir()
	e := filepath.Join(d, "e")
	os.WriteFile(e, nil, 0o644)
	if xorFile(e, filepath.Join(d, "o"), dataKey) == nil {
		t.Fatal("empty input should fail")
	}
}
