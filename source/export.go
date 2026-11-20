package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ExportOpts describes "export the reference tables" for one game install (used by the translation team).
type ExportOpts struct {
	Game   string // game folder (the one with SoulWorker64.dll and datas\)
	Region string // jp, kr or gb (global)
	Data   string // data12 (default) or data11 for gb
	PwFile string // text file holding the archive password (jp has a built-in one)
	Out    string // output root, default <app>\reference
	Keep   bool
}

// ExportRef collects what the translation work needs from one game install: the table layout (schema),
// the string/script tables, the game version and fingerprints. Nothing in the game folder is changed.
func (a *App) ExportRef(o ExportOpts) int {
	start := time.Now()
	_ = os.MkdirAll(a.logsDir(), 0o755)
	lf, _ := os.Create(filepath.Join(a.logsDir(), "export_"+start.Format("20060102_150405")+".log"))
	if lf != nil {
		defer lf.Close()
	}
	log := &Logger{f: lf}
	fail := func(format string, args ...any) int {
		log.Fail(format, args...)
		return 1
	}

	region := strings.ToLower(strings.TrimSpace(o.Region))
	if region == "global" || region == "gl" {
		region = "gb"
	}
	if region != "jp" && region != "kr" && region != "gb" {
		return fail("region must be jp, kr or gb (got %q)", o.Region)
	}
	data := o.Data
	if data == "" {
		data = "data12"
		if region == "gb" {
			data = "data11"
		}
	}
	game := strings.TrimSpace(o.Game)
	if !exists(filepath.Join(game, "SoulWorker64.dll")) {
		return fail("SoulWorker64.dll not found in %q - give the game folder with -game", game)
	}
	if !exists(filepath.Join(game, "datas", data+".v")) {
		return fail("%s.v not found in %s", data, filepath.Join(game, "datas"))
	}
	pass := ""
	if o.PwFile != "" {
		b, err := os.ReadFile(o.PwFile)
		if err != nil {
			return fail("cannot read the password file: %v", err)
		}
		pass = parsePasswordFile(string(b), data)
		if pass == "" {
			return fail("no password for %s found in the password file (expected a line like \"%s = ...\")", data, data)
		}
	} else if pw, _ := a.archivePassword(data); pw != "" {
		pass = pw
	} else {
		return fail("no archive password: give -pwfile <text file with the password> (or add a key line, see the README)")
	}

	ver := "0.0.0.0"
	if kv := readKV(filepath.Join(game, "Ver.ini")); kv["ver"] != "" {
		ver = kv["ver"]
	}
	out := o.Out
	if out == "" {
		out = filepath.Join(a.dir, "reference")
	}
	dest := filepath.Join(out, strings.ToUpper(region), ver)
	log.Info("Exporting %s, game version %s -> %s", strings.ToUpper(region), ver, dest)

	cfg := *a.cfg
	cfg.SWPath, cfg.DataSource, cfg.Converter = game, data, "builtin"
	r := &Run{a: a, cfg: &cfg, log: log, work: filepath.Join(a.dir, "work"), pass: pass}
	_ = os.RemoveAll(r.work)
	if err := os.MkdirAll(r.work, 0o755); err != nil {
		return fail("cannot create work folder: %v", err)
	}
	if !o.Keep {
		defer os.RemoveAll(r.work)
	}
	for _, s := range []func() error{r.stepSchema, r.stepSource} {
		if err := s(); err != nil {
			return fail("%v", err)
		}
	}

	log.Info("Extracting the tables")
	ex := filepath.Join(r.work, "ex")
	res, err := r.sevenZ([]string{"x", r.zipPath, "-p" + r.password(), "-o" + ex, "-y", "-r"}, r.work, nil)
	if err != nil {
		return fail("%v", err)
	}
	if res.Code >= 2 {
		return fail("7z could not extract the archive (code %d) - is the password right?\n%s", res.Code, tailLines(res.Out, 6))
	}
	tdir := filepath.Join(ex, "bin", "Table")
	ents, err := os.ReadDir(tdir)
	if err != nil {
		return fail("archive has no bin\\Table folder: %v", err)
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return fail("%v", err)
	}
	var names []string
	for _, e := range ents {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".res") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	groups := exportSelect(region, names)
	var sums []string
	total := 0
	for _, g := range groups {
		gdir := filepath.Join(dest, g.dir)
		_ = os.RemoveAll(gdir)
		if err := os.MkdirAll(gdir, 0o755); err != nil {
			return fail("%v", err)
		}
		for _, n := range g.files {
			dst := filepath.Join(gdir, n)
			if err := copyFile(filepath.Join(tdir, n), dst); err != nil {
				return fail("%v", err)
			}
			h, _ := hashFile(dst)
			sums = append(sums, fmt.Sprintf("%s  %s/%s", h, g.dir, n))
			total++
		}
		log.OK("%s: %d table(s)", g.dir, len(g.files))
	}
	if total == 0 {
		return fail("no matching tables found in the archive")
	}
	if err := copyFile(r.schema, filepath.Join(dest, "schema.json")); err != nil {
		return fail("%v", err)
	}
	if exists(filepath.Join(game, "Ver.ini")) {
		_ = copyFile(filepath.Join(game, "Ver.ini"), filepath.Join(dest, "Ver.ini"))
	}
	dll, _ := hashFile(filepath.Join(game, "SoulWorker64.dll"))
	build := fmt.Sprintf("region: %s\nversion: %s\ndate: %s\nexporter: SW TL Updater %s\ndata file: %s.v\ndata file sha256: %s\nSoulWorker64.dll sha256: %s\ntables: %d\n",
		strings.ToUpper(region), ver, time.Now().Format("2006-01-02 15:04"), version, data, r.srcSHA, dll, total)
	_ = os.WriteFile(filepath.Join(dest, "build.txt"), []byte(build), 0o644)
	_ = os.WriteFile(filepath.Join(dest, "files.sha256"), []byte(strings.Join(sums, "\n")+"\n"), 0o644)
	log.OK("Done in %s: %d table(s), schema.json, Ver.ini, build.txt -> %s", time.Since(start).Round(time.Second), total, dest)
	return 0
}

// parsePasswordFile finds the password of one data file in a password list like
//
//	.data12 = old password        <- a leading dot marks an old password: ignored
//	data12 = current password
//
// If the file has no "name = value" lines for that data file, its first non-empty line is the password.
func parsePasswordFile(text, data string) string {
	var plain, found string
	for _, ln := range strings.Split(strings.TrimPrefix(text, "\xEF\xBB\xBF"), "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, ".") {
			continue
		}
		if i := strings.Index(ln, "="); i > 0 {
			name := strings.TrimSpace(ln[:i])
			if strings.EqualFold(name, data) {
				found = strings.TrimSpace(ln[i+1:]) // the newest one (last line) wins
			}
			continue
		}
		if plain == "" && !strings.Contains(ln, " ") {
			plain = ln
		}
	}
	if found != "" {
		return found
	}
	return plain
}

type exportGroup struct {
	dir   string
	files []string
}

// exportSelect picks the tables the translation needs (same rules as the old Get-res batch file).
func exportSelect(region string, names []string) []exportGroup {
	lower := func(s string) string { return strings.ToLower(s) }
	if region == "gb" {
		var gs []exportGroup
		for _, lang := range []string{"ENG", "TWN", "CHN"} {
			suf := "_" + lower(lang) + ".res"
			skip := map[string]bool{"tb_ui_img": true, "tb_help": true, "tb_help_popup": true}
			if lang == "CHN" {
				for _, x := range []string{"tb_check_attendance_info", "tb_check_attendance_reward", "tb_make", "tb_shop"} {
					skip[x] = true
				}
			}
			g := exportGroup{dir: lang + "-res"}
			for _, n := range names {
				ln := lower(n)
				if strings.HasSuffix(ln, suf) && !skip[strings.TrimSuffix(ln, suf)] {
					g.files = append(g.files, n)
				}
			}
			if len(g.files) > 0 {
				gs = append(gs, g)
			}
		}
		return gs
	}
	g := exportGroup{dir: "res"}
	for _, n := range names {
		ln := lower(n)
		base := strings.TrimSuffix(ln, ".res")
		ok := strings.HasSuffix(base, "_string") || strings.HasSuffix(base, "_script") || (region == "jp" && base == "tb_systemmail")
		if region == "jp" && (base == "tb_option_string" || base == "tb_loading_string" || base == "tb_talk_string") {
			ok = false
		}
		if ok {
			g.files = append(g.files, n)
		}
	}
	return []exportGroup{g}
}

var _ = errors.New
