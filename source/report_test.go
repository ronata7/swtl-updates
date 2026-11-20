package main

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"swtlupdater/internal/archivekey"
)

func readZipText(t *testing.T, zb []byte) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(zb), int64(len(zb)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range zr.File {
		r, _ := f.Open()
		b, _ := io.ReadAll(r)
		r.Close()
		out[f.Name] = string(b)
	}
	return out
}

func TestMakeReport(t *testing.T) {
	dir := t.TempDir()
	game := filepath.Join(dir, "game")
	os.MkdirAll(filepath.Join(game, "datas"), 0o755)
	os.WriteFile(filepath.Join(game, "SoulWorker64.dll"), []byte("dll"), 0o644)
	os.WriteFile(filepath.Join(game, "datas", "data12.v"), []byte("data"), 0o644)
	os.WriteFile(filepath.Join(game, "Ver.ini"), []byte("[ver]\r\nver=1.2.3.4\r\n"), 0o644)
	os.MkdirAll(filepath.Join(dir, "logs"), 0o755)
	os.WriteFile(filepath.Join(dir, "logs", "update_20260101_000000.log"), []byte("old log"), 0o644)
	os.WriteFile(filepath.Join(dir, "logs", "online_update.log"), []byte("online"), 0o644)
	tl := filepath.Join(dir, "TL csv")
	os.MkdirAll(tl, 0o755)
	os.WriteFile(filepath.Join(tl, "tb_A.csv"), []byte("h\r\n1\tx\r\n"), 0o644)
	key, _ := archivekey.Encode("data12", "fake-secret-pass")
	ini := filepath.Join(dir, "SWTLUpdater.ini")
	os.WriteFile(ini, []byte("GameFolder="+game+"\r\nArchiveKey="+key+"\r\n"), 0o644)
	app := &App{dir: dir, cfgPath: ini, cfg: &Config{SWPath: game, TLPath: tl, DataSource: "data12", DataOutput: "data12", Converter: "builtin", ArchiveKey: key}}
	if app.MakeReport() != 0 {
		t.Fatal("report failed")
	}
	ms, _ := filepath.Glob(filepath.Join(dir, "reports", "report_*.zip"))
	if len(ms) != 1 {
		t.Fatalf("reports: %v", ms)
	}
	zb, _ := os.ReadFile(ms[0])
	files := readZipText(t, zb)
	for _, n := range []string{"info.txt", "SWTLUpdater.ini", "logs/update_20260101_000000.log", "logs/online_update.log"} {
		if _, ok := files[n]; !ok {
			t.Fatalf("missing %s in %v", n, files)
		}
	}
	for n, s := range files {
		if strings.Contains(s, "fake-secret-pass") || strings.Contains(s, key) {
			t.Fatalf("the key is in %s", n)
		}
	}
	if !strings.Contains(files["info.txt"], "ver=1.2.3.4") || !strings.Contains(files["info.txt"], "tb_A.csv") || !strings.Contains(files["info.txt"], "Archive key     : found") {
		t.Fatalf("info.txt: %s", files["info.txt"])
	}
	if !strings.Contains(files["SWTLUpdater.ini"], "ArchiveKey=(hidden)") {
		t.Fatal("key line not hidden in the settings copy")
	}
}

func TestWritePartsAndJoin(t *testing.T) {
	dir := t.TempDir()
	data := bytes.Repeat([]byte("abcdefghij"), 25) // 250 bytes
	one, err := writeParts(filepath.Join(dir, "a.zip"), data, 1000)
	if err != nil || len(one) != 1 {
		t.Fatalf("%v %v", err, one)
	}
	parts, err := writeParts(filepath.Join(dir, "b.zip"), data, 100)
	if err != nil || len(parts) != 3 || !strings.HasSuffix(parts[0], "b.zip.001") {
		t.Fatalf("%v %v", err, parts)
	}
	var joined []byte
	for _, p := range parts {
		b, _ := os.ReadFile(p)
		joined = append(joined, b...)
	}
	if !bytes.Equal(joined, data) {
		t.Fatal("parts do not join back to the original")
	}
}
