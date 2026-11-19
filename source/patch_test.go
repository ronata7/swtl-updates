package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"swtlupdater/internal/rowpatch"
)

func hx(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func bigTable(n int, edit map[int]string) []byte {
	var sb strings.Builder
	sb.WriteString("A\tB\r\n")
	for i := 0; i < n; i++ {
		t := fmt.Sprintf("Line %d of the table with some words <br> and more", i)
		if e, ok := edit[i]; ok {
			t = e
		}
		fmt.Fprintf(&sb, "%d\t%s\r\n", i, t)
	}
	return []byte(sb.String())
}

type patchEnv struct {
	dir, target string
	oldB, newB  []byte
	file        ManFile
	srv         *httptest.Server
	hits        map[string]int
	patchBytes  []byte
}

func newPatchEnv(t *testing.T) *patchEnv {
	e := &patchEnv{hits: map[string]int{}}
	e.dir = t.TempDir()
	e.oldB = bigTable(2000, nil)
	e.newB = bigTable(2000, map[int]string{5: "changed five", 1999: "changed last"})
	e.target = filepath.Join(e.dir, "TL csv", "tb_X.csv")
	os.MkdirAll(filepath.Dir(e.target), 0o755)
	os.WriteFile(e.target, e.oldB, 0o644)
	e.patchBytes = rowpatch.Gzip(rowpatch.Make(e.oldB, e.newB))
	e.file = ManFile{Path: "TL csv/tb_X.csv", Size: int64(len(e.newB)), SHA256: hx(e.newB), Kind: "translation",
		Patches: []ManPatch{{From: hx(e.oldB), Path: "patches/TL csv/tb_X.csv.aaaaaaaaaaaa.rp", Size: int64(len(e.patchBytes)), SHA256: hx(e.patchBytes)}}}
	e.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		e.hits[r.URL.Path]++
		if strings.HasSuffix(r.URL.Path, ".rp") {
			w.Write(e.patchBytes)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(e.srv.Close)
	return e
}

func (e *patchEnv) try() (string, error) {
	dst := filepath.Join(e.dir, "out.part")
	err := tryPatch(e.srv.Client(), e.srv.URL+"/", e.target, dst, e.file, &Logger{})
	b, _ := os.ReadFile(dst)
	return string(b), err
}

func TestPatchApplies(t *testing.T) {
	e := newPatchEnv(t)
	got, err := e.try()
	if err != nil || got != string(e.newB) {
		t.Fatalf("err=%v match=%v", err, got == string(e.newB))
	}
	if len(e.patchBytes) > 400 {
		t.Fatalf("patch is %d bytes", len(e.patchBytes))
	}
}

func TestPatchNotForThisVersion(t *testing.T) {
	e := newPatchEnv(t)
	os.WriteFile(e.target, bigTable(2000, map[int]string{7: "player edited"}), 0o644)
	_, err := e.try()
	if err == nil || !strings.HasPrefix(err.Error(), "no patch") {
		t.Fatalf("want 'no patch', got %v", err)
	}
	if len(e.hits) != 0 {
		t.Fatal("nothing should be downloaded when no patch fits")
	}
}

func TestPatchTampered(t *testing.T) {
	e := newPatchEnv(t)
	e.patchBytes = append([]byte(nil), e.patchBytes...)
	e.patchBytes[len(e.patchBytes)/2] ^= 0xFF // corrupt after the manifest hash was taken
	if _, err := e.try(); err == nil {
		t.Fatal("tampered patch must be refused")
	}
}

func TestPatchWrongResultRefused(t *testing.T) {
	e := newPatchEnv(t)
	// a validly signed hash of the patch, but the patch builds something else than the signed file
	other := bigTable(2000, map[int]string{5: "evil text"})
	e.patchBytes = rowpatch.Gzip(rowpatch.Make(e.oldB, other))
	e.file.Patches[0].Size = int64(len(e.patchBytes))
	e.file.Patches[0].SHA256 = hx(e.patchBytes)
	if _, err := e.try(); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("want result mismatch, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(e.dir, "out.part")); err == nil {
		t.Fatal("a wrong result must not be written")
	}
}

func TestPatchPathsAndLimits(t *testing.T) {
	for _, p := range []string{"patches/../x.rp", "/patches/x.rp", "patches/a\\b.rp", "patches/C:x.rp", "files/x.rp", "patches/x.csv", "patches//x.rp"} {
		if safePatchPath(p) {
			t.Errorf("%q should be refused", p)
		}
	}
	if !safePatchPath("patches/TL csv/tb_X.csv.0123456789ab.rp") {
		t.Error("normal patch path refused")
	}
	e := newPatchEnv(t)
	e.file.Patches[0].Path = "patches/../../evil.rp"
	if _, err := e.try(); err == nil {
		t.Fatal("bad patch path must be refused")
	}
	e = newPatchEnv(t)
	e.file.Patches[0].Size = maxPatchBytes + 1
	if _, err := e.try(); err == nil {
		t.Fatal("huge patch must be refused")
	}
}

func TestPatchServerDown(t *testing.T) {
	e := newPatchEnv(t)
	e.srv.Close()
	if _, err := e.try(); err == nil {
		t.Fatal("download failure must be an error (caller falls back to the whole file)")
	}
}
