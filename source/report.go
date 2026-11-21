package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// reportPartSize: a report bigger than this is cut into parts (report_x.zip.001, .002, ...) because chat
// uploads have a size and a file-count limit. Join them again by putting all parts in one folder and opening
// the .001 file with 7-Zip.
var reportPartSize int64 = 8 << 20

// redact removes the Windows user name from text that is going to be sent to someone else.
func redact(s string) string {
	for _, h := range []string{os.Getenv("USERPROFILE"), homeDir()} {
		if len(h) > 3 {
			s = strings.ReplaceAll(s, h, `C:\Users\<user>`)
		}
	}
	if u := os.Getenv("USERNAME"); len(u) > 2 {
		s = strings.ReplaceAll(s, `\`+u+`\`, `\<user>\`)
	}
	return s
}

func homeDir() string { h, _ := os.UserHomeDir(); return h }

// MakeReport collects what is needed to find a problem into one file in the reports folder: recent logs,
// the settings (without the archive key), versions and fingerprints of the game files and the translation.
func (a *App) MakeReport() int {
	fmt.Println(" Making a report - this takes a few seconds...")
	type entry struct {
		name string
		data []byte
	}
	var es []entry
	add := func(name string, data []byte) { es = append(es, entry{name, data}) }
	addFile := func(name, path string, max int64) {
		if fi, err := os.Stat(path); err == nil && !fi.IsDir() && fi.Size() <= max {
			if b, err := os.ReadFile(path); err == nil {
				add(name, b)
			}
		}
	}
	newest := func(dir, pattern string, n int) []string {
		ms, _ := filepath.Glob(filepath.Join(dir, pattern))
		sort.Slice(ms, func(i, j int) bool { return ms[i] > ms[j] }) // names carry the time stamp
		if len(ms) > n {
			ms = ms[:n]
		}
		return ms
	}

	// ---- info.txt
	var in bytes.Buffer
	p := func(format string, args ...any) { fmt.Fprintf(&in, format+"\r\n", args...) }
	p("SW TL Updater report  (%s)", time.Now().Format("2006-01-02 15:04:05"))
	p("Program version : %s", version)
	p("System          : %s/%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		if out, err := exec.Command("cmd", "/c", "ver").Output(); err == nil {
			p("Windows         : %s", strings.TrimSpace(string(out)))
		}
	}
	c := a.cfg
	p("Program folder  : %s", redact(a.dir))
	p("Game folder     : %s  (exists: %v)", redact(c.SWPath), isDir(c.SWPath))
	p("Translation dir : %s  (exists: %v)", redact(c.TLPath), isDir(c.TLPath))
	p("Patcher folder  : %s  (exists: %v)", redact(c.PatcherPath), isDir(c.PatcherPath))
	p("Data file       : %s -> %s    Converter: %s", c.DataSource, c.DataOutput, c.Converter)
	p("Update address  : %s", c.UpdateURL)
	p("Translation addr: %s", c.TranslationURL)
	p("Update at start : %v", c.UpdateAtStart)
	p("Archive key     : %s", map[bool]string{true: "found", false: "NOT found"}[a.hasArchiveKey()])
	p("Game running    : %v     SWPatcher running: %v", processRunning("SoulWorker100.exe") || processRunning("SoulWorker.exe"), processRunning("SWPatcher.exe"))
	p("")
	if b, err := os.ReadFile(filepath.Join(c.SWPath, "Ver.ini")); err == nil {
		p("Game Ver.ini:\r\n%s", strings.TrimSpace(string(b)))
	} else {
		p("Game Ver.ini    : not readable")
	}
	fileLine := func(label, path string) {
		fi, err := os.Stat(path)
		if err != nil {
			p("%-15s: not found (%s)", label, redact(path))
			return
		}
		h, _ := hashFile(path)
		p("%-15s: %d bytes, modified %s, sha256 %s", label, fi.Size(), fi.ModTime().Format("2006-01-02 15:04"), h)
	}
	fileLine("SoulWorker64.dll", filepath.Join(c.SWPath, "SoulWorker64.dll"))
	fileLine(c.DataSource+".v", a.gameDataFile())
	fileLine("patched "+c.DataOutput+".v", filepath.Join(a.dir, c.DataOutput+".v"))
	if c.PatcherPath != "" {
		fileLine("in patcher folder", filepath.Join(c.PatcherPath, c.DataOutput+".v"))
	}
	if b, err := os.ReadFile(a.statePath()); err == nil {
		p("\r\nLast patch record:\r\n%s", strings.TrimSpace(string(b)))
	}
	p("\r\nTranslation files (%s):", redact(c.TLPath))
	tl, _ := filepath.Glob(filepath.Join(c.TLPath, "*.csv"))
	sort.Strings(tl)
	for _, f := range tl {
		fi, _ := os.Stat(f)
		h, _ := hashFile(f)
		p("  %-34s %10d bytes  %s", filepath.Base(f), fi.Size(), h)
	}
	if len(tl) == 0 {
		p("  (no .csv files found)")
	}
	add("info.txt", in.Bytes())

	// ---- settings without the key
	if b, err := os.ReadFile(a.cfgPath); err == nil {
		var out []string
		for _, ln := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(ln)), "archivekey") {
				ln = "ArchiveKey=(hidden)"
			}
			out = append(out, redact(ln))
		}
		add("SWTLUpdater.ini", []byte(strings.Join(out, "\r\n")))
	}

	// ---- logs and small helpers
	for _, pat := range []string{"update_*.log", "export_*.log", "session_*.log"} {
		for _, f := range newest(a.logsDir(), pat, 6) {
			addFile("logs/"+filepath.Base(f), f, 12<<20)
		}
	}
	addFile("logs/online_update.log", filepath.Join(a.logsDir(), "online_update.log"), 2<<20)
	if ms, _ := filepath.Glob(filepath.Join(a.logsDir(), "new_rows", "*.csv")); len(ms) > 0 {
		for _, f := range ms {
			addFile("logs/new_rows/"+filepath.Base(f), f, 2<<20)
		}
	}
	for _, f := range newest(filepath.Join(a.dir, "cache"), "schema_*.json", 1) {
		addFile("cache/"+filepath.Base(f), f, 6<<20)
	}

	// ---- write the zip
	dir := filepath.Join(a.dir, "reports")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Println("  x cannot create the reports folder:", err)
		return 1
	}
	zipPath := filepath.Join(dir, "report_"+time.Now().Format("20060102_150405")+".zip")
	var zb bytes.Buffer
	zw := zip.NewWriter(&zb)
	for _, e := range es {
		w, err := zw.Create(e.name)
		if err != nil {
			fmt.Println("  x", err)
			return 1
		}
		if _, err := w.Write(e.data); err != nil {
			fmt.Println("  x", err)
			return 1
		}
	}
	if err := zw.Close(); err != nil {
		fmt.Println("  x", err)
		return 1
	}
	files, err := writeParts(zipPath, zb.Bytes(), reportPartSize)
	if err != nil {
		fmt.Println("  x cannot write the report:", err)
		return 1
	}
	fmt.Printf("\n Report ready (%d KB, %d file(s) inside):\n", zb.Len()/1024, len(es))
	for _, f := range files {
		fmt.Println("   " + f)
	}
	if len(files) > 1 {
		fmt.Println(" It is cut into parts. Send all of them together; open the .001 part with 7-Zip to read it.")
	}
	fmt.Println(" The archive key is NOT included, and your Windows user name is hidden. Send the file to ronata on Discord.")
	if runtime.GOOS == "windows" {
		_ = exec.Command("explorer", "/select,"+files[0]).Start()
	}
	return 0
}

// writeParts saves data as one file, or as numbered parts when it is larger than size.
func writeParts(path string, data []byte, size int64) ([]string, error) {
	if int64(len(data)) <= size {
		return []string{path}, os.WriteFile(path, data, 0o644)
	}
	var out []string
	for i, off := 1, int64(0); off < int64(len(data)); i, off = i+1, off+size {
		end := off + size
		if end > int64(len(data)) {
			end = int64(len(data))
		}
		name := fmt.Sprintf("%s.%03d", path, i)
		if err := os.WriteFile(name, data[off:end], 0o644); err != nil {
			return out, err
		}
		out = append(out, name)
	}
	return out, nil
}

func (a *App) hasArchiveKey() bool {
	pw, _ := a.archivePassword(a.cfg.DataSource)
	return pw != ""
}

var _ = io.EOF
