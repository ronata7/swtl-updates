package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type RunOpts struct {
	Dry, KeepWork, Strict, Interactive bool
}

var errNothing = errors.New("nothing changed")
var errCancelled = errors.New("cancelled")

type Run struct {
	a   *App
	cfg *Config
	opt RunOpts
	log *Logger

	work    string
	zipPath string
	schema  string
	resDir  string
	before  map[string]string
	changed []string
	newV    string
	exDir   string
}

const totalSteps = 7

func (a *App) RunPatch(opt RunOpts) (code int) {
	start := time.Now()
	defer setTitle(appTitle())
	_ = os.MkdirAll(a.logsDir(), 0o755)
	logPath := filepath.Join(a.logsDir(), "update_"+start.Format("20060102_150405")+".log")
	lf, _ := os.Create(logPath)
	log := &Logger{f: lf}
	defer func() {
		if lf != nil {
			lf.Close()
		}
	}()

	r := &Run{a: a, cfg: a.cfg, opt: opt, log: log, work: filepath.Join(a.dir, "work")}
	_ = os.RemoveAll(r.work)
	if err := os.MkdirAll(r.work, 0o755); err != nil {
		log.Fail("cannot create work folder: %v", err)
		return 1
	}

	steps := []func() error{r.preflight, r.stepSchema, r.stepSource, r.stepExtract, r.stepConvert, r.stepRepack, r.stepPublish}
	for _, s := range steps {
		if err := s(); err != nil {
			if errors.Is(err, errCancelled) {
				log.Info("Cancelled - nothing was changed.")
				_ = os.RemoveAll(r.work)
				return 0
			}
			if errors.Is(err, errNothing) {
				log.Warn("No tables changed - the CSVs already match the game data (or none map to a table). Nothing was written.")
				_ = os.RemoveAll(r.work)
				log.Info("Log: %s", logPath)
				return 3
			}
			log.Fail("%v", err)
			log.Info("Work folder kept for inspection: %s", r.work)
			log.Info("Log: %s", logPath)
			log.Info("Need help? Send this log to Discord: ronata")
			return 1
		}
	}
	if !opt.KeepWork {
		_ = os.RemoveAll(r.work)
	}
	fmt.Println()
	if opt.Dry {
		log.OK("DRY RUN finished in %s - %d table(s) would change, nothing was written.", time.Since(start).Round(time.Second), len(r.changed))
	} else {
		log.OK("Patching complete in %s - %d table(s) updated.", time.Since(start).Round(time.Second), len(r.changed))
	}
	if n := len(log.Warns); n > 0 {
		fmt.Printf("\n  Warnings (%d):\n", n)
		for _, w := range log.Warns {
			fmt.Printf("   - %s\n", w)
		}
	}
	log.Info("Log: %s", logPath)
	return 0
}

// ---- 1. preflight -------------------------------------------------------

func (r *Run) preflight() error {
	r.log.Step(1, totalSteps, "Checking settings, tools and translation files")
	if err := r.cfg.Validate(); err != nil {
		return err
	}
	need := []string{"7z.exe", "7z.dll", "SW Res structs dumper.exe", "SW Res structs dumper.pdb", "Zydis.dll"}
	var missing []string
	for _, n := range need {
		if !exists(r.toolPath(n)) {
			missing = append(missing, n)
		}
	}
	if r.cfg.Converter != "builtin" {
		for _, n := range []string{"SWAdmin.Cli.exe", "SWAdmin.Core.dll", "Newtonsoft.Json.dll"} {
			if !exists(r.toolPath("SWAdmin", n)) {
				missing = append(missing, "SWAdmin\\"+n)
			}
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("Tools folder is incomplete, missing: %s\n%s", strings.Join(missing, ", "), toolsHelp)
	}

	names, problems := ScanCSVs(r.cfg.TLPath)
	if len(names) == 0 {
		return fmt.Errorf("no .csv files in %s", r.cfg.TLPath)
	}
	r.log.OK("%d translation CSV file(s) found", len(names))
	if len(problems) > 0 {
		for _, p := range problems {
			if r.opt.Strict {
				r.log.Fail("%s", p)
			} else {
				r.log.Warn("CSV: %s", p)
			}
		}
		if r.opt.Strict {
			return errors.New("CSV problems found (strict mode)")
		}
	}

	if !isDir(r.cfg.PatcherPath) {
		r.log.Warn("Patcher folder not found (%s) - the result will only be saved next to the updater", r.cfg.PatcherPath)
	}
	if !r.a.guardProcesses(r.log, r.opt.Interactive) {
		return errCancelled
	}
	return nil
}

// ---- 2. schema ----------------------------------------------------------

func (r *Run) stepSchema() error {
	r.log.Step(2, totalSteps, "Preparing table schema")
	dll := filepath.Join(r.cfg.SWPath, "SoulWorker64.dll")
	h := ""
	files := []string{dll, r.toolPath("SW Res structs dumper.exe")}
	mode := r.cfg.Converter
	if mode != "builtin" {
		files = append(files, r.toolPath("SWAdmin", "SWAdmin.Cli.exe"), r.toolPath("SWAdmin", "SWAdmin.Core.dll"))
	} else {
		h = "builtin-schema-1"
	}
	if mode == "check" {
		h += "check-1" // first run in check mode dumps once more so both schema builders can be compared
	}
	for _, p := range files {
		x, err := hashFile(p)
		if err != nil {
			return err
		}
		h += x
	}
	key := shortHash(h)
	cache := filepath.Join(r.a.dir, "cache", "schema_"+key+".json")
	r.schema = filepath.Join(r.work, "schema.json")
	if exists(cache) {
		r.log.OK("Game build unchanged - reusing cached schema (%s)", key)
		return copyFile(cache, r.schema)
	}

	r.log.Info("New game build or tool version - dumping structures (about a minute)...")
	dump := filepath.Join(r.work, "dump")
	if err := os.MkdirAll(dump, 0o755); err != nil {
		return err
	}
	dumper := []string{"SW Res structs dumper.exe", "SW Res structs dumper.pdb", "Zydis.dll"}
	for _, n := range dumper {
		if err := copyFile(r.toolPath(n), filepath.Join(dump, n)); err != nil {
			return err
		}
	}
	if _, err := r.tool(filepath.Join(dump, dumper[0]), []string{dll}, dump, nil); err != nil {
		return fmt.Errorf("could not start the structs dumper: %v", err)
	}
	for _, n := range dumper {
		_ = os.Remove(filepath.Join(dump, n))
	}
	ents, _ := os.ReadDir(dump)
	n := 0
	var dumpDirs []string
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		d := filepath.Join(dump, e.Name())
		cs, _ := filepath.Glob(filepath.Join(d, "*.cs"))
		if len(cs) == 0 {
			return fmt.Errorf("dumper folder %s has no .cs files - game version may be unsupported by the dumper", e.Name())
		}
		dumpDirs = append(dumpDirs, d)
		if mode == "builtin" {
			n++
			continue
		}
		// SWAdmin merges into the existing schema file, so every folder is applied in turn.
		res, err := r.tool(r.toolPath("SWAdmin", "SWAdmin.Cli.exe"), []string{"-dumper_to_schema", "-di", d, "-fo", r.schema}, r.work, nil)
		if err != nil {
			return err
		}
		if res.Code != 0 {
			r.log.Warn("SWAdmin schema step exited with code %d for %s", res.Code, e.Name())
		}
		n++
	}
	if mode == "builtin" || mode == "check" {
		es, err := dumperToSchema(dumpDirs...)
		if err != nil {
			if mode == "builtin" {
				return fmt.Errorf("could not build the schema: %v", err)
			}
			r.log.Warn("Cross-check: built-in schema builder failed: %v", err)
		} else {
			target := r.schema
			if mode == "check" {
				target = filepath.Join(r.work, "schema_builtin.json")
			}
			if err := writeSchema(target, es); err != nil {
				return err
			}
			if mode == "check" {
				if ok, why := sameSchema(target, r.schema); ok {
					r.log.OK("Cross-check: built-in schema is identical to SWAdmin's (%d tables)", len(es))
				} else {
					r.log.Warn("Cross-check: built-in schema DIFFERS from SWAdmin's: %s - please send this log to Discord: ronata", why)
				}
			}
		}
	}
	if n == 0 {
		return errors.New("the structs dumper produced no output - check that the game version is supported")
	}
	if sz, err := fileSize(r.schema); err != nil || sz == 0 {
		return errors.New("schema.json was not created - check the SWAdmin tools and game version")
	}
	if err := copyFile(r.schema, cache); err != nil {
		r.log.Warn("could not cache schema: %v", err)
	}
	r.log.OK("Schema created and cached (%s)", key)
	return nil
}

func shortHash(s string) string {
	// s is a concatenation of sha256 hex strings; fold into a short key.
	var x uint64 = 1469598103934665603
	for i := 0; i < len(s); i++ {
		x ^= uint64(s[i])
		x *= 1099511628211
	}
	return fmt.Sprintf("%012x", x&0xffffffffffff)
}

// ---- 3. source ----------------------------------------------------------

func (r *Run) stepSource() error {
	r.log.Step(3, totalSteps, "Reading %s.v from the game folder", r.cfg.DataSource)
	src := filepath.Join(r.cfg.SWPath, "datas", r.cfg.DataSource+".v")
	cp := filepath.Join(r.work, r.cfg.DataSource+"-Copy.v")
	if err := copyFile(src, cp); err != nil {
		return err
	}
	if !isXoredZip(cp) {
		return fmt.Errorf("%s.v does not look like the expected XOR-0x55 archive - game format may have changed", r.cfg.DataSource)
	}
	r.zipPath = filepath.Join(r.work, r.cfg.DataSource+".zip")
	if err := r.xor(cp, r.zipPath); err != nil {
		return err
	}
	_ = os.Remove(cp)
	r.log.OK("Decrypted")
	return nil
}

// ---- 4. extract ---------------------------------------------------------

func (r *Run) stepExtract() error {
	r.log.Step(4, totalSteps, "Extracting string and script tables")
	ex := filepath.Join(r.work, "ex")
	res, err := r.sevenZ([]string{"x", r.zipPath, "-p" + zipPass, "-o" + ex, "-y", "-r"}, r.work, nil)
	if err != nil {
		return err
	}
	if res.Code >= 2 {
		return fmt.Errorf("7z could not extract the archive (code %d):\n%s", res.Code, tailLines(res.Out, 8))
	}
	tdir := filepath.Join(ex, "bin", "Table")
	ents, err := os.ReadDir(tdir)
	if err != nil {
		return fmt.Errorf("archive has no bin\\Table folder: %v", err)
	}
	excl := map[string]bool{}
	for _, e := range r.cfg.ExcludeTables {
		excl[strings.ToLower(e)] = true
	}
	r.resDir = filepath.Join(r.work, "res")
	r.before = map[string]string{}
	haveMail := false
	for _, e := range ents {
		n := e.Name()
		ln := strings.ToLower(n)
		if e.IsDir() || !strings.HasSuffix(ln, ".res") {
			continue
		}
		base := strings.TrimSuffix(ln, ".res")
		if !(strings.HasSuffix(base, "_string") || strings.HasSuffix(base, "_script") || base == "tb_systemmail") || excl[base] {
			continue
		}
		if base == "tb_systemmail" {
			haveMail = true
		}
		dst := filepath.Join(r.resDir, n)
		if err := copyFile(filepath.Join(tdir, n), dst); err != nil {
			return err
		}
		h, err := hashFile(dst)
		if err != nil {
			return err
		}
		r.before[n] = h
	}
	if len(r.before) == 0 {
		return errors.New("no string/script tables found in the archive")
	}
	if !haveMail {
		r.log.Warn("tb_SystemMail.res not found in the archive")
	}
	r.exDir = ex
	r.log.OK("%d table(s) selected", len(r.before))
	return nil
}

// ---- 5. convert ---------------------------------------------------------

func (r *Run) stepConvert() error {
	r.log.Step(5, totalSteps, "Applying translations")
	mode := r.cfg.Converter
	if mode == "" {
		mode = "swadmin"
	}
	var out string
	var code int
	var shown int
	warn := func(s string) {
		if shown < 10 {
			r.log.Warn("%s", s)
		}
		shown++
	}
	var names []string
	for n := range r.before {
		names = append(names, n)
	}
	sort.Strings(names)

	if mode == "builtin" {
		done, problems, err := convertCSVDir(r.schema, r.cfg.TLPath, r.resDir, names)
		if err != nil {
			return err
		}
		for _, pr := range problems {
			if strings.Contains(pr, "error") {
				warn("Converter: " + pr)
				code = 1
			}
		}
		out = fmt.Sprintf("converted %d file(s)", len(done))
		if shown > 0 {
			out += "\n" + strings.Join(problems, "\n")
		}
	} else {
		res, err := r.tool(r.toolPath("SWAdmin", "SWAdmin.Cli.exe"),
			[]string{"-csv_to_res", "-schema_file", r.schema, "-di", r.cfg.TLPath, "-do", r.resDir}, r.work, nil)
		if err != nil {
			return err
		}
		out, code = res.Out, res.Code
		for _, ln := range strings.Split(res.Out, "\n") {
			l := strings.ToLower(ln)
			if strings.Contains(l, "error") || strings.Contains(l, "exception") || strings.Contains(l, "fail") {
				warn("SWAdmin: " + strings.TrimSpace(ln))
			}
		}
		if code != 0 {
			r.log.Warn("SWAdmin exited with code %d", code)
		}
		if mode == "check" {
			r.crossCheck(names)
		}
	}
	res := struct {
		Out  string
		Code int
	}{out, code}

	r.changed = nil
	for n, old := range r.before {
		h, err := hashFile(filepath.Join(r.resDir, n))
		if err != nil {
			return err
		}
		if h != old {
			r.changed = append(r.changed, n)
		}
	}
	sort.Strings(r.changed)
	if len(r.changed) == 0 {
		if res.Code != 0 || shown > 0 {
			return errors.New("SWAdmin reported problems and no table changed:\n" + tailLines(res.Out, 8))
		}
		return errNothing
	}
	r.log.OK("%d of %d table(s) changed", len(r.changed), len(r.before))
	for _, n := range r.changed {
		r.log.Info("  %s", strings.TrimSuffix(n, ".res"))
	}
	return nil
}

// ---- 6. repack ----------------------------------------------------------

func (r *Run) entry(n string) string { return filepath.Join("bin", "Table", n) }

func (r *Run) testZip() error {
	res, err := r.sevenZ([]string{"t", r.zipPath, "-p" + zipPass}, r.work, nil)
	if err != nil {
		return err
	}
	if res.Code >= 2 || !strings.Contains(res.Out, "Everything is Ok") {
		return fmt.Errorf("archive integrity test failed:\n%s", tailLines(res.Out, 6))
	}
	return nil
}

// verifyZip extracts the whole rebuilt archive and compares every file with the
// tree we meant to write (original files unchanged, changed tables translated).
func (r *Run) verifyZip() error {
	vd := filepath.Join(r.work, "verify")
	_ = os.RemoveAll(vd)
	res, err := r.sevenZ([]string{"x", r.zipPath, "-p" + zipPass, "-o" + vd, "-y", "-r"}, r.work, nil)
	if err != nil {
		return err
	}
	if res.Code >= 2 {
		return fmt.Errorf("could not read back the rebuilt archive:\n%s", tailLines(res.Out, 6))
	}
	n := 0
	err = filepath.Walk(r.exDir, func(p string, info os.FileInfo, werr error) error {
		if werr != nil || info.IsDir() {
			return werr
		}
		rel, _ := filepath.Rel(r.exDir, p)
		want, err := hashFile(p)
		if err != nil {
			return err
		}
		got, err := hashFile(filepath.Join(vd, rel))
		if err != nil || got != want {
			return fmt.Errorf("%s in the rebuilt archive does not match what was intended", rel)
		}
		n++
		return nil
	})
	if err != nil {
		return err
	}
	_ = os.RemoveAll(vd)
	r.log.Info("Read-back check: %d files identical to the intended content", n)
	return nil
}

type zentry struct {
	raw, norm, method string
	enc               bool
}

// listZip returns every file entry of the archive: the exact name stored in it
// (raw, e.g. "..\\bin\\Table\\x.res") and a normalised lower-case form.
func (r *Run) listZip(zip string) ([]zentry, error) {
	res, err := r.sevenZ([]string{"l", zip, "-slt", "-p" + zipPass}, r.work, nil)
	if err != nil {
		return nil, err
	}
	if res.Code >= 2 {
		return nil, fmt.Errorf("7z could not list the archive (code %d)", res.Code)
	}
	var out []zentry
	inList := false
	cur, isDir, method, enc := "", false, "", false
	flush := func() {
		if cur != "" && !isDir {
			out = append(out, zentry{cur, strings.ToLower(strings.ReplaceAll(cur, "\\", "/")), method, enc})
		}
		cur, isDir, method, enc = "", false, "", false
	}
	for _, ln := range strings.Split(res.Out, "\n") {
		ln = strings.TrimRight(ln, "\r")
		if strings.HasPrefix(ln, "----------") {
			inList = true
			continue
		}
		if !inList {
			continue
		}
		switch {
		case strings.HasPrefix(ln, "Path = "):
			flush()
			cur = strings.TrimPrefix(ln, "Path = ")
		case ln == "Folder = +":
			isDir = true
		case strings.HasPrefix(ln, "Method = "):
			method = strings.TrimPrefix(ln, "Method = ")
		case ln == "Encrypted = +":
			enc = true
		}
	}
	flush()
	sort.Slice(out, func(i, j int) bool { return out[i].norm < out[j].norm })
	return out, nil
}

// checkEntries makes sure the archive holds exactly the original set of files:
// nothing lost, nothing duplicated, nothing added under a different name.
func (r *Run) checkEntries(want []zentry) error {
	got, err := r.listZip(r.zipPath)
	if err != nil {
		return err
	}
	seen := map[string]int{}
	dups := []string{}
	for _, g := range got {
		seen[g.norm]++
		if seen[g.norm] == 2 {
			dups = append(dups, g.norm)
		}
	}
	wantSet := map[string]bool{}
	missing := []string{}
	for _, w := range want {
		wantSet[w.norm] = true
		if seen[w.norm] == 0 {
			missing = append(missing, w.norm)
		}
	}
	extra := []string{}
	for _, g := range got {
		if !wantSet[g.norm] {
			extra = append(extra, g.norm)
		}
	}
	if len(got) == len(want) && len(dups) == 0 && len(missing) == 0 && len(extra) == 0 {
		return nil
	}
	ex := func(l []string) string {
		if len(l) > 3 {
			return strings.Join(l[:3], ", ") + ", ..."
		}
		return strings.Join(l, ", ")
	}
	return fmt.Errorf("archive has %d entries, expected %d (%d duplicated: %s; %d missing: %s; %d unexpected: %s)",
		len(got), len(want), len(dups), ex(dups), len(missing), ex(missing), len(extra), ex(extra))
}

// checkMethods confirms every replaced table is stored like the originals:
// password-protected with classic ZipCrypto (the game cannot read AES), Deflate or Store.
func (r *Run) checkMethods() error {
	got, err := r.listZip(r.zipPath)
	if err != nil {
		return err
	}
	want := map[string]bool{}
	for _, n := range r.changed {
		want["bin/table/"+strings.ToLower(n)] = true
	}
	seen := 0
	for _, e := range got {
		base := e.norm
		if i := strings.Index(base, "bin/table/"); i >= 0 {
			base = base[i:]
		}
		if !want[base] {
			continue
		}
		seen++
		if !e.enc || !strings.HasPrefix(e.method, "ZipCrypto") || !(strings.HasSuffix(e.method, "Deflate") || strings.HasSuffix(e.method, "Store")) {
			return fmt.Errorf("%s was stored as %q (encrypted=%v), expected ZipCrypto Deflate", e.raw, e.method, e.enc)
		}
	}
	if seen != len(want) {
		return fmt.Errorf("could not confirm storage method for %d of %d replaced tables", len(want)-seen, len(want))
	}
	return nil
}

func (r *Run) stepRepack() error {
	r.log.Step(6, totalSteps, "Rebuilding %s.v", r.cfg.DataSource)
	orig, err := r.listZip(r.zipPath)
	if err != nil {
		return err
	}
	if len(orig) == 0 {
		return errors.New("could not read the entry list of the original archive")
	}
	// Each table is replaced under the exact name the archive already uses
	// (the game's archive stores them as "..\bin\Table\x.res"), so 7z updates
	// the existing entry instead of adding a second one.
	for _, n := range r.changed {
		if err := copyFile(filepath.Join(r.resDir, n), filepath.Join(r.exDir, r.entry(n))); err != nil {
			return err
		}
		suffix := "bin/table/" + strings.ToLower(n)
		raw := ""
		for _, e := range orig {
			if e.norm == suffix || strings.HasSuffix(e.norm, "/"+suffix) {
				raw = e.raw
				break
			}
		}
		if raw == "" {
			return fmt.Errorf("table %s is not in the original archive - refusing to add it under a guessed name", n)
		}
		f, err := os.Open(filepath.Join(r.resDir, n))
		if err != nil {
			return err
		}
		res, err := r.sevenZ([]string{"u", r.zipPath, "-mm=Deflate", "-mem=ZipCrypto", "-mx=5", "-mtc=on", "-mta=on", "-si" + raw, "-p" + zipPass}, r.work, f)
		f.Close()
		if err != nil {
			return err
		}
		if res.Code >= 2 {
			return fmt.Errorf("7z update of %s failed (code %d):\n%s", n, res.Code, tailLines(res.Out, 6))
		}
	}
	if err := r.checkEntries(orig); err != nil {
		return err
	}
	if err := r.checkMethods(); err != nil {
		return err
	}
	if err := r.testZip(); err != nil {
		return err
	}
	if err := r.verifyZip(); err != nil {
		return err
	}
	r.log.OK("Archive updated: %d entries, all original files kept, %d table(s) replaced", len(orig), len(r.changed))

	r.newV = filepath.Join(r.work, r.cfg.DataOutput+".v")
	if err := r.xor(r.zipPath, r.newV); err != nil {
		return err
	}
	if !isXoredZip(r.newV) {
		return errors.New("encrypted output failed its header check")
	}
	r.log.OK("Encrypted")
	return nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// ---- 7. publish ---------------------------------------------------------

func (r *Run) stepPublish() error {
	r.log.Step(7, totalSteps, "Saving result")
	outFile := filepath.Join(r.a.dir, r.cfg.DataOutput+".v")
	patcherFile := filepath.Join(r.cfg.PatcherPath, r.cfg.DataOutput+".v")
	havePatcher := isDir(r.cfg.PatcherPath)

	if r.opt.Dry {
		r.log.Info("Would save: %s", outFile)
		if havePatcher {
			r.log.Info("Would save: %s", patcherFile)
		}
		return nil
	}

	// back up whatever is about to be replaced
	prev := patcherFile
	if !havePatcher || !exists(prev) {
		prev = outFile
	}
	if exists(prev) {
		bk := filepath.Join(r.a.backupDir(), fmt.Sprintf("%s_%s.v", r.cfg.DataOutput, time.Now().Format("20060102_150405")))
		if err := copyFile(prev, bk); err != nil {
			return fmt.Errorf("could not back up the previous file: %v", err)
		}
		r.log.OK("Previous version backed up: %s", filepath.Base(bk))
		r.a.pruneBackups(r.log)
	}

	if err := atomicCopy(r.newV, outFile); err != nil {
		return err
	}
	r.log.OK("Saved %s", outFile)
	if havePatcher {
		if err := atomicCopy(r.newV, patcherFile); err != nil {
			return err
		}
		r.log.OK("Copied to patcher: %s", patcherFile)
	}
	return nil
}

// crossCheck (Converter=check) converts again with the built-in code and compares every table with
// what SWAdmin just wrote. The SWAdmin result is what gets used.
func (r *Run) crossCheck(names []string) {
	alt := filepath.Join(r.work, "res_builtin")
	done, problems, err := convertCSVDir(r.schema, r.cfg.TLPath, alt, names)
	if err != nil {
		r.log.Warn("Cross-check could not run: %v", err)
		return
	}
	same, diff, explained := 0, 0, 0
	for _, n := range done {
		a, e1 := os.ReadFile(filepath.Join(r.resDir, n))
		b, e2 := os.ReadFile(filepath.Join(alt, n))
		if e1 != nil || e2 != nil || !bytes.Equal(a, b) {
			diff++
			r.log.Warn("Cross-check: %s DIFFERS between SWAdmin and the built-in converter (%d vs %d bytes) - please send this log to Discord: ronata", n, len(a), len(b))
			if e1 == nil && e2 == nil && explained < 3 {
				explained++
				r.log.Info("    %s", describeResDiff(a, b))
			}
		} else {
			same++
		}
	}
	for _, pr := range problems {
		if strings.Contains(pr, "error") {
			r.log.Warn("Cross-check: built-in converter: %s", pr)
			diff++
		}
	}
	if diff == 0 {
		r.log.OK("Cross-check: all %d table(s) are identical to SWAdmin's result", same)
	} else {
		r.log.Warn("Cross-check: %d table(s) identical, %d with differences", same, diff)
	}
}

// describeResDiff says where two versions of a table differ (diagnostics for the cross-check).
func describeResDiff(a, b []byte) string {
	first, count := -1, 0
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			if first < 0 {
				first = i
			}
			count++
		}
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "first difference at byte %d, %d differing byte(s); ", first, count)
	rowsEnd := len(a) - 34
	if first >= rowsEnd && len(a) == len(b) {
		sb.WriteString("row data identical, only the checksum differs. ")
	} else {
		sb.WriteString("the row data differs. ")
		if first >= 0 {
			lo := first - 6
			if lo < 0 {
				lo = 0
			}
			hi := first + 10
			if hi > len(a) {
				hi = len(a)
			}
			if hi > len(b) {
				hi = len(b)
			}
			fmt.Fprintf(&sb, "SWAdmin bytes % x vs built-in % x. ", a[lo:hi], b[lo:hi])
		}
	}
	if len(a) > 34 && len(b) > 34 {
		fa, fb := string(a[len(a)-32:]), string(b[len(b)-32:])
		var sum uint64
		for _, c := range b[4 : len(b)-34] {
			sum += uint64(c)
		}
		fmt.Fprintf(&sb, "checksums SWAdmin=%s built-in=%s (our byte sum %d)", fa, fb, sum)
	}
	return sb.String()
}
