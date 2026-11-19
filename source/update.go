package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"swtlupdater/internal/rowpatch"
	"time"
)

// Online updates. The update server only needs to serve static files (a public GitHub repo,
// GitHub Pages, any web host). Everything is trusted only if the manifest carries a valid
// Ed25519 signature made with the publisher's private key - so a hacked host cannot push
// anything. Files are also size- and SHA-256-checked and only allowed folders can be touched.

const (
	maxFileBytes     = 300 << 20
	maxManifestBytes = 4 << 20
)

// ManFile / Manifest: keep in sync with cmd/swtlpublish.
type ManFile struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	Kind   string `json:"kind"`
	// Patches: small downloads that turn an older copy of this file into this one (see rowpatch).
	Patches []ManPatch `json:"patches,omitempty"`
}

// ManPatch turns the local file whose SHA-256 is From into the file of the entry it belongs to.
type ManPatch struct {
	From   string `json:"from"`
	Path   string `json:"path"` // below patches/ on the update server
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type Manifest struct {
	Version  string    `json:"version"`
	Released string    `json:"released"`
	Notes    string    `json:"notes"`
	Files    []ManFile `json:"files"`
	Delete   []string  `json:"delete"`
}

// updSource is one place updates come from. The main source carries the program, the readme and
// Tools (and the translation too, unless a separate translation source is configured); the
// translation source may only ever touch "TL csv/".
type updSource struct {
	label string
	url   string
	key   string
	seen  string
	tl    bool
}

type UpdatePlan struct {
	M          *Manifest
	Seen       string
	Download   []ManFile
	Delete     []string
	SelfUpdate bool
}

func checkURL(base string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(base))
	if err != nil || u.Host == "" {
		return "", errors.New("the update address is not a valid web address")
	}
	h := u.Hostname()
	if u.Scheme != "https" && !(u.Scheme == "http" && (h == "127.0.0.1" || h == "localhost")) {
		return "", errors.New("the update address must start with https://")
	}
	s := u.String()
	if !strings.HasSuffix(s, "/") {
		s += "/"
	}
	return s, nil
}

func httpGet(c *http.Client, u string, max int64) ([]byte, error) {
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "SWTLUpdater/"+version)
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("server answered HTTP %d for %s", resp.StatusCode, u)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, errors.New("download is unexpectedly large")
	}
	return b, nil
}

func fetchManifest(c *http.Client, base, key string) (*Manifest, error) {
	pub, err := base64.StdEncoding.DecodeString(strings.TrimSpace(key))
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return nil, errors.New("the update key in the settings is not valid")
	}
	mb, err := httpGet(c, base+"manifest.json", maxManifestBytes)
	if err != nil {
		return nil, err
	}
	sb, err := httpGet(c, base+"manifest.json.sig", 4096)
	if err != nil {
		return nil, err
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sb)))
	if err != nil || !ed25519.Verify(ed25519.PublicKey(pub), mb, sig) {
		return nil, errors.New("signature check FAILED - the update files are not from the publisher, nothing was changed")
	}
	var m Manifest
	if err := json.Unmarshal(mb, &m); err != nil {
		return nil, fmt.Errorf("update list is unreadable: %v", err)
	}
	return &m, nil
}

// safeRel accepts only plain relative paths inside the folders the updater manages.
func safeRel(p string) (string, bool) {
	if p == "" || strings.ContainsAny(p, "\\:") || strings.HasPrefix(p, "/") {
		return "", false
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return "", false
		}
	}
	if p != "SWTLUpdater.exe" && p != "README.txt" && !strings.HasPrefix(p, "Tools/") && !strings.HasPrefix(p, "TL csv/") {
		return "", false
	}
	return filepath.FromSlash(p), true
}

func cmpVersion(a, b string) int {
	pa, pb := strings.Split(strings.TrimPrefix(a, "v"), "."), strings.Split(strings.TrimPrefix(b, "v"), ".")
	for i := 0; i < 4; i++ {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			if x > y {
				return 1
			}
			return -1
		}
	}
	return 0
}

func (a *App) seenPath(name string) string { return filepath.Join(a.dir, "cache", name) }

func isTLPath(p string) bool { return strings.HasPrefix(p, "TL csv/") }

func (a *App) planUpdate(m *Manifest, self string, src updSource) (*UpdatePlan, error) {
	seenPath := a.seenPath(src.seen)
	if seen, err := os.ReadFile(seenPath); err == nil && cmpVersion(m.Version, strings.TrimSpace(string(seen))) < 0 {
		return nil, fmt.Errorf("the server offers version %s, which is older than %s that was already seen - refusing to go back", m.Version, strings.TrimSpace(string(seen)))
	}
	plan := &UpdatePlan{M: m, Seen: seenPath}
	splitTL := !src.tl && strings.TrimSpace(a.cfg.TranslationURL) != "" // translation comes from its own source
	for _, f := range m.Files {
		rel, ok := safeRel(f.Path)
		if !ok || (src.tl && !isTLPath(f.Path)) {
			return nil, fmt.Errorf("update list contains a path that is not allowed: %q", f.Path)
		}
		if splitTL && isTLPath(f.Path) {
			continue
		}
		if len(f.SHA256) != 64 || f.Size < 0 || f.Size > maxFileBytes {
			return nil, fmt.Errorf("update list entry for %s is invalid", f.Path)
		}
		target := filepath.Join(a.dir, rel)
		isApp := f.Path == "SWTLUpdater.exe"
		if isApp {
			target = self // the exe may have been renamed by the user
			if a.selfDone || cmpVersion(m.Version, version) <= 0 {
				continue
			}
		}
		if h, err := hashFile(target); err == nil && strings.EqualFold(h, f.SHA256) {
			continue
		}
		plan.Download = append(plan.Download, f)
		if isApp {
			plan.SelfUpdate = true
		}
	}
	for _, d := range m.Delete {
		rel, ok := safeRel(d)
		if !ok || d == "SWTLUpdater.exe" || (src.tl && !isTLPath(d)) {
			return nil, fmt.Errorf("update list asks to delete a path that is not allowed: %q", d)
		}
		if splitTL && isTLPath(d) {
			continue
		}
		if exists(filepath.Join(a.dir, rel)) {
			plan.Delete = append(plan.Delete, d)
		}
	}
	return plan, nil
}

func escapePath(p string) string {
	segs := strings.Split(p, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return strings.Join(segs, "/")
}

func downloadVerified(c *http.Client, u, dst string, f ManFile) error {
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "SWTLUpdater/"+version)
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("%s: server answered HTTP %d", f.Path, resp.StatusCode)
	}
	mode := os.FileMode(0o644)
	if strings.HasSuffix(strings.ToLower(f.Path), ".exe") {
		mode = 0o755
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, h), io.LimitReader(resp.Body, f.Size+1))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("%s: %v", f.Path, err)
	}
	if n != f.Size || !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), f.SHA256) {
		return fmt.Errorf("%s: downloaded file does not match the signed update list (corrupt or tampered)", f.Path)
	}
	return nil
}

// safePatchPath allows only plain paths below patches/ (no .., no drive letters, no backslashes).
func safePatchPath(p string) bool {
	if !strings.HasPrefix(p, "patches/") || strings.ContainsAny(p, "\\:") || strings.Contains(p, "..") || strings.Contains(p, "//") {
		return false
	}
	return strings.HasSuffix(p, ".rp") && len(p) < 300
}

const maxPatchBytes = 64 << 20

// tryPatch rebuilds f from the local copy at target plus a small downloaded patch. Any problem returns
// an error and the caller downloads the whole file instead - nothing is trusted until the result
// matches the signed SHA-256 and size.
func tryPatch(c *http.Client, base, target, dst string, f ManFile, log *Logger) error {
	if len(f.Patches) == 0 {
		return errors.New("no patch offered")
	}
	local, err := os.ReadFile(target)
	if err != nil {
		return errors.New("no patch: the file is not on this PC yet")
	}
	sum := sha256.Sum256(local)
	have := hex.EncodeToString(sum[:])
	var pt *ManPatch
	for i := range f.Patches {
		if strings.EqualFold(f.Patches[i].From, have) {
			pt = &f.Patches[i]
			break
		}
	}
	if pt == nil {
		return errors.New("no patch for the version on this PC")
	}
	if !safePatchPath(pt.Path) || pt.Size <= 0 || pt.Size > maxPatchBytes || len(pt.SHA256) != 64 {
		return errors.New("patch entry is not valid")
	}
	z, err := httpGet(c, base+escapePath(pt.Path), pt.Size)
	if err != nil {
		return err
	}
	zs := sha256.Sum256(z)
	if int64(len(z)) != pt.Size || !strings.EqualFold(hex.EncodeToString(zs[:]), pt.SHA256) {
		return errors.New("patch does not match the signed update list")
	}
	text, err := rowpatch.Gunzip(z, 2*f.Size+(1<<20))
	if err != nil {
		return err
	}
	out, err := rowpatch.Apply(local, text)
	if err != nil {
		return err
	}
	rs := sha256.Sum256(out)
	if int64(len(out)) != f.Size || !strings.EqualFold(hex.EncodeToString(rs[:]), f.SHA256) {
		return errors.New("patched file does not match the signed update list")
	}
	if err := os.WriteFile(dst, out, 0o644); err != nil {
		return err
	}
	log.Info("   patch: downloaded %d KB (the whole file is %d KB)", (pt.Size+1023)/1024, (f.Size+1023)/1024)
	return nil
}

// applyUpdate downloads and checks everything first; files are only replaced when all of it is good.
func (a *App) applyUpdate(c *http.Client, base string, plan *UpdatePlan, self string, log *Logger) error {
	tmp := filepath.Join(a.dir, "update_tmp")
	_ = os.RemoveAll(tmp)
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	parts := make([]string, len(plan.Download))
	for i, f := range plan.Download {
		log.Info("Downloading %s  (%d/%d)", f.Path, i+1, len(plan.Download))
		parts[i] = filepath.Join(tmp, fmt.Sprintf("%04d.part", i))
		if rel, ok := safeRel(f.Path); ok && len(f.Patches) > 0 && f.Path != "SWTLUpdater.exe" {
			if err := tryPatch(c, base, filepath.Join(a.dir, rel), parts[i], f, log); err == nil {
				continue
			} else if !strings.HasPrefix(err.Error(), "no patch") {
				log.Warn("   patch not used (%v) - downloading the whole file", err)
			}
		}
		if err := downloadVerified(c, base+"files/"+escapePath(f.Path), parts[i], f); err != nil {
			return err
		}
	}
	selfPart := ""
	for i, f := range plan.Download {
		if f.Path == "SWTLUpdater.exe" {
			selfPart = parts[i]
			continue
		}
		rel, _ := safeRel(f.Path)
		target := filepath.Join(a.dir, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.Rename(parts[i], target); err != nil {
			return fmt.Errorf("could not replace %s (is it in use?): %v", f.Path, err)
		}
	}
	for _, d := range plan.Delete {
		rel, _ := safeRel(d)
		_ = os.Remove(filepath.Join(a.dir, rel))
	}
	if selfPart != "" {
		old := self + ".old"
		_ = os.Remove(old)
		if err := os.Rename(self, old); err != nil { // a running exe can be renamed, not overwritten
			return fmt.Errorf("could not replace the program file: %v", err)
		}
		if err := os.Rename(selfPart, self); err != nil {
			_ = os.Rename(old, self)
			return fmt.Errorf("could not put the new program file in place: %v", err)
		}
	}
	if selfPart != "" {
		a.selfDone = true
	}
	_ = os.MkdirAll(filepath.Dir(plan.Seen), 0o755)
	_ = os.WriteFile(plan.Seen, []byte(plan.M.Version), 0o644)
	return nil
}

func countKinds(fs []ManFile) string {
	n := map[string]int{}
	for _, f := range fs {
		n[f.Kind]++
	}
	var parts []string
	if n["app"] > 0 {
		parts = append(parts, "the program itself")
	}
	if n["translation"] > 0 {
		parts = append(parts, fmt.Sprintf("%d translation file(s)", n["translation"]))
	}
	if n["tools"] > 0 {
		parts = append(parts, fmt.Sprintf("%d tool file(s)", n["tools"]))
	}
	if n["docs"] > 0 {
		parts = append(parts, "the readme")
	}
	return strings.Join(parts, ", ")
}

// Update checks the update server(s) and installs what is new. It returns true if the program
// replaced itself and started the new copy - the caller must then exit immediately.
// startup=true: quiet on "nothing to do" and on network trouble.
// The translation source (if configured) is handled first, so a program restart happens last.
func (a *App) Update(interactive, startup bool, args []string, relaunch bool) (restarted bool, err error) {
	log := &Logger{}
	c := a.cfg
	if strings.TrimSpace(c.UpdateURL) == "" || strings.TrimSpace(c.UpdateKey) == "" {
		if !startup {
			log.Warn("Online updates are not set up. Put the update address and key from your download in Settings.")
		}
		return false, nil
	}
	if !startup {
		fmt.Println("\n Checking for updates...")
	}
	var firstErr error
	if strings.TrimSpace(c.TranslationURL) != "" {
		key := strings.TrimSpace(c.TranslationKey)
		if key == "" {
			key = c.UpdateKey
		}
		_, e := a.updateOne(updSource{label: "Translation", url: c.TranslationURL, key: key, seen: "update_seen_translation.txt", tl: true}, interactive, startup, args, false, log)
		if e != nil {
			firstErr = fmt.Errorf("translation source: %v", e)
		}
	}
	restarted, e := a.updateOne(updSource{label: "Program", url: c.UpdateURL, key: c.UpdateKey, seen: "update_seen.txt"}, interactive, startup, args, relaunch, log)
	if e != nil {
		if firstErr != nil {
			return restarted, fmt.Errorf("%v; program source: %v", firstErr, e)
		}
		return restarted, e
	}
	return restarted, firstErr
}

func (a *App) updateOne(src updSource, interactive, startup bool, args []string, relaunch bool, log *Logger) (restarted bool, err error) {
	base, err := checkURL(src.url)
	if err != nil {
		return false, err
	}
	timeout := 60 * time.Second
	if startup {
		timeout = 8 * time.Second
	}
	cl := &http.Client{Timeout: timeout}
	self := a.self
	m, err := fetchManifest(cl, base, src.key)
	if err != nil {
		if startup && !strings.Contains(err.Error(), "signature") {
			fmt.Println(" (Could not check for updates - offline? Continuing.)")
			return false, nil
		}
		return false, err
	}
	plan, err := a.planUpdate(m, self, src)
	if err != nil {
		return false, err
	}
	if len(plan.Download) == 0 && len(plan.Delete) == 0 {
		_ = os.MkdirAll(filepath.Dir(plan.Seen), 0o755)
		_ = os.WriteFile(plan.Seen, []byte(m.Version), 0o644)
		if !startup {
			log.OK("%s: everything is up to date (published version %s).", src.label, m.Version)
		}
		return false, nil
	}
	fmt.Printf("\n %s update available (version %s): %s\n", src.label, m.Version, countKinds(plan.Download))
	if len(plan.Delete) > 0 {
		fmt.Printf(" %d outdated file(s) will be removed.\n", len(plan.Delete))
	}
	if strings.TrimSpace(m.Notes) != "" {
		fmt.Printf(" What's new: %s\n", strings.TrimSpace(m.Notes))
	}
	if interactive {
		ans := strings.ToLower(prompt(" Install now? [Y/n] "))
		if ans != "" && ans != "y" && ans != "yes" {
			fmt.Println(" Skipped.")
			return false, nil
		}
	}
	if err := a.applyUpdate(cl, base, plan, self, log); err != nil {
		return false, err
	}
	log.OK("Update installed.")
	if plan.SelfUpdate && !relaunch {
		fmt.Println(" The new version of the program will be used the next time you start it.")
	}
	if plan.SelfUpdate && relaunch {
		fmt.Println(" Restarting the new version...")
		var rest []string
		for _, x := range args {
			if x != "--update" {
				rest = append(rest, x)
			}
		}
		cmd := exec.Command(self, rest...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		cmd.Env = append(os.Environ(), "SWTL_RELAUNCH=1")
		if err := cmd.Start(); err != nil {
			return false, fmt.Errorf("updated, but could not restart automatically (%v) - please start the program again", err)
		}
		return true, nil
	}
	return false, nil
}
