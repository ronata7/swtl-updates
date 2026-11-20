// swtlpublish - publisher tool for SW TL Updater online updates. Runs on the publisher's PC.
//
//	swtlpublish keygen  [-out DIR]
//	swtlpublish publish -src DIR -out DIR -key FILE -version X.Y.Z [-notes "text"] [-part all|main|translation]
//	swtlpublish pack    -src DIR -url https://.../ -pub PUBLICKEY -zip FILE
//	swtlpublish verify  -out DIR -pub PUBLICKEY
//
// -src is a folder laid out like the program folder: SWTLUpdater.exe, README.txt, Tools\..., TL csv\...
// Upload the contents of publish's -out folder to the update server (e.g. a public GitHub repo).
package main

import (
	"archive/zip"
	"bufio"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"swtlupdater/internal/archivekey"
	"swtlupdater/internal/rowpatch"
)

// keep in sync with update.go in the main program
type ManFile struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	Kind   string `json:"kind"`
	// Patches lets a player who already has an older copy of this file download a small patch instead.
	Patches []ManPatch `json:"patches,omitempty"`
}

// ManPatch turns the file whose SHA-256 is From into the file this entry describes.
type ManPatch struct {
	From   string `json:"from"`
	Path   string `json:"path"` // under patches/ in the update folder
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

func die(f string, a ...any) { fmt.Fprintf(os.Stderr, "error: "+f+"\n", a...); os.Exit(1) }

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: swtlpublish keygen | publish | pack | verify   (add -h after a command for its options)")
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	switch cmd {
	case "keygen":
		keygen(args)
	case "publish":
		publish(args)
	case "pack":
		pack(args)
	case "verify":
		verify(args)
	case "mkkey":
		mkkey(args)
	default:
		die("unknown command %q", cmd)
	}
}

// mkkey turns a game archive password into an encrypted key line for README.txt (or the ArchiveKey= setting).
// The password is typed (or piped) in, never given on the command line, so it stays out of the shell history.
func mkkey(args []string) {
	if len(args) != 1 {
		die("usage: swtlpublish mkkey <data file name, e.g. data12>   (then type the password)")
	}
	fmt.Fprintln(os.Stderr, "Archive password for "+args[0]+" (type it, then press Enter):")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	line = strings.TrimSpace(line)
	out, err := archivekey.Encode(args[0], line)
	if err != nil {
		die("%v", err)
	}
	if back, pw, err := archivekey.Decode(out); err != nil || back != strings.ToLower(args[0]) || pw != line {
		die("self-check failed")
	}
	fmt.Println(out)
	fmt.Fprintln(os.Stderr, "\nAdd this line to the end of README.txt (a newer line for the same data file replaces an older one),")
	fmt.Fprintln(os.Stderr, "or give it to a player to paste into SWTLUpdater.ini as  ArchiveKey=<the line>")
}

func keygen(args []string) {
	fs := flag.NewFlagSet("keygen", flag.ExitOnError)
	out := fs.String("out", ".", "folder to write the key files into")
	fs.Parse(args)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		die("%v", err)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		die("%v", err)
	}
	privPath := filepath.Join(*out, "swtl_update_PRIVATE.key")
	if _, err := os.Stat(privPath); err == nil {
		die("%s already exists - refusing to overwrite your existing key", privPath)
	}
	if err := os.WriteFile(privPath, []byte(base64.StdEncoding.EncodeToString(priv)+"\n"), 0o600); err != nil {
		die("%v", err)
	}
	pubS := base64.StdEncoding.EncodeToString(pub)
	_ = os.WriteFile(filepath.Join(*out, "swtl_update_public.txt"), []byte(pubS+"\n"), 0o644)
	fmt.Println("Key pair created.")
	fmt.Println("  PRIVATE key:", privPath, " <- keep it secret and backed up. Never upload it. Anyone with it can publish updates.")
	fmt.Println("  PUBLIC key :", pubS)
	fmt.Println("The public key goes to the players (the 'pack' command puts it in their settings file).")
}

func kindOf(p string) string {
	switch {
	case p == "SWTLUpdater.exe":
		return "app"
	case p == "README.txt":
		return "docs"
	case strings.HasPrefix(p, "Tools/"):
		return "tools"
	default:
		return "translation"
	}
}

func scan(src string) ([]ManFile, map[string]string) {
	var files []ManFile
	abs := map[string]string{}
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		rel = filepath.ToSlash(rel)
		if rel != "SWTLUpdater.exe" && rel != "README.txt" && !strings.HasPrefix(rel, "Tools/") && !strings.HasPrefix(rel, "TL csv/") {
			return nil // never publish settings, logs, backups, work files...
		}
		if strings.HasSuffix(rel, ".old") || strings.HasSuffix(rel, ".part") {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		h := sha256.New()
		n, err := io.Copy(h, f)
		if err != nil {
			return err
		}
		files = append(files, ManFile{Path: rel, Size: n, SHA256: hex.EncodeToString(h.Sum(nil)), Kind: kindOf(rel)})
		abs[rel] = p
		return nil
	})
	if err != nil {
		die("%v", err)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, abs
}

func loadPriv(path string) ed25519.PrivateKey {
	b, err := os.ReadFile(path)
	if err != nil {
		die("cannot read key: %v", err)
	}
	k, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
	if err != nil || len(k) != ed25519.PrivateKeySize {
		die("%s is not a valid private key file", path)
	}
	return ed25519.PrivateKey(k)
}

func copyTo(src, dst string) {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		die("%v", err)
	}
	in, err := os.Open(src)
	if err != nil {
		die("%v", err)
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		die("%v", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		die("%v", err)
	}
	out.Close()
}

func publish(args []string) {
	fs := flag.NewFlagSet("publish", flag.ExitOnError)
	src := fs.String("src", "", "folder laid out like the program folder (exe, Tools, TL csv, README.txt)")
	out := fs.String("out", "", "folder to write the update files into (upload this to the server)")
	keyF := fs.String("key", "", "private key file made by 'keygen'")
	ver := fs.String("version", "", "version number to publish, e.g. 2.6.0 (must be higher than the last one for the exe to update)")
	notes := fs.String("notes", "", "short 'what's new' text shown to players")
	part := fs.String("part", "all", "all = everything; main = program, readme, Tools (no translation); translation = only 'TL csv' (for a separate translation repo)")
	histF := fs.String("hist", "", "folder that keeps older versions of big tables for making patches (default: swtl_history next to -out). Keep it between releases.")
	patchMin := fs.Int64("patchmin", 256<<10, "make patches only for translation files at least this many bytes")
	patchKeep := fs.Int("patchkeep", 10, "how many older versions get a patch to the newest one (0 = no patches)")
	fs.Parse(args)
	if *src == "" || *out == "" || *keyF == "" || *ver == "" {
		die("need -src, -out, -key and -version")
	}
	if *part != "all" && *part != "main" && *part != "translation" {
		die("-part must be all, main or translation")
	}
	priv := loadPriv(*keyF)
	files, abs := scan(*src)
	if *part != "all" {
		var keep []ManFile
		for _, f := range files {
			isTL := strings.HasPrefix(f.Path, "TL csv/")
			if (*part == "translation") == isTL {
				keep = append(keep, f)
			}
		}
		files = keep
	}
	if len(files) == 0 {
		die("no publishable files found in %s", *src)
	}
	hasApp := false
	for _, f := range files {
		if f.Path == "SWTLUpdater.exe" {
			hasApp = true
		}
	}
	if !hasApp {
		fmt.Println("note: no SWTLUpdater.exe in -src, so this release updates only tools/translations.")
	}
	// same files as the release already in -out? then leave it alone (nothing to push for this part)
	if pb, err := os.ReadFile(filepath.Join(*out, "manifest.json")); err == nil {
		var prev Manifest
		if _, serr := os.Stat(filepath.Join(*out, "manifest.json.sig")); serr == nil && json.Unmarshal(pb, &prev) == nil && len(prev.Files) == len(files) {
			now := map[string]ManFile{}
			for _, f := range files {
				now[f.Path] = f
			}
			same := true
			for _, f := range prev.Files {
				if n, ok := now[f.Path]; !ok || n.SHA256 != f.SHA256 || n.Size != f.Size {
					same = false
					break
				}
			}
			if same {
				fmt.Printf("UNCHANGED (%s): the files are identical to the release already in %s (version %s). Left as it is - nothing to push for this part.\n", *part, *out, prev.Version)
				return
			}
		}
	}
	m := Manifest{Version: *ver, Released: time.Now().UTC().Format(time.RFC3339), Notes: *notes, Files: files, Delete: []string{}}

	// files that were in the previous release but are gone now get deleted on the players' PCs
	if pb, err := os.ReadFile(filepath.Join(*out, "manifest.json")); err == nil {
		var prev Manifest
		if json.Unmarshal(pb, &prev) == nil {
			now := map[string]bool{}
			for _, f := range files {
				now[f.Path] = true
			}
			for _, f := range prev.Files {
				if *part == "main" && strings.HasPrefix(f.Path, "TL csv/") {
					continue // translation files moved to the other source: never tell old players to delete them
				}
				if !now[f.Path] && f.Path != "SWTLUpdater.exe" {
					m.Delete = append(m.Delete, f.Path)
				}
			}
		}
	}
	// patches for big tables: remember the old copy that is still in the update folder, then build
	// patches from the last few versions to the new one
	if *patchKeep > 0 {
		hist := *histF
		if hist == "" {
			ao, _ := filepath.Abs(*out)
			hist = filepath.Join(filepath.Dir(ao), "swtl_history", filepath.Base(ao))
		}
		_ = os.RemoveAll(filepath.Join(*out, "patches"))
		makePatches(&m, abs, *out, hist, *patchMin, *patchKeep)
	}
	// refresh the files folder
	_ = os.RemoveAll(filepath.Join(*out, "files"))
	for _, f := range files {
		copyTo(abs[f.Path], filepath.Join(*out, "files", filepath.FromSlash(f.Path)))
	}
	mb, _ := json.MarshalIndent(m, "", "  ")
	sig := ed25519.Sign(priv, mb)
	_ = os.MkdirAll(*out, 0o755)
	if err := os.WriteFile(filepath.Join(*out, "manifest.json"), mb, 0o644); err != nil {
		die("%v", err)
	}
	if err := os.WriteFile(filepath.Join(*out, "manifest.json.sig"), []byte(base64.StdEncoding.EncodeToString(sig)+"\n"), 0o644); err != nil {
		die("%v", err)
	}
	fmt.Printf("Published version %s: %d file(s), %d to delete.\n", *ver, len(files), len(m.Delete))
	fmt.Println("Upload everything inside", *out, "(manifest.json, manifest.json.sig and the files folder) to your update server.")
}

func readGz(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return rowpatch.Gunzip(b, 1<<31)
}

func sha(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

// makePatches fills m.Files[i].Patches and writes the patch files into out/patches.
func makePatches(m *Manifest, abs map[string]string, out, hist string, minSize int64, keep int) {
	var prevReleased time.Time
	if pb, err := os.ReadFile(filepath.Join(out, "manifest.json")); err == nil {
		var prev Manifest
		if json.Unmarshal(pb, &prev) == nil {
			prevReleased, _ = time.Parse(time.RFC3339, prev.Released)
		}
	}
	if prevReleased.IsZero() {
		prevReleased = time.Now().Add(-time.Hour)
	}
	total, made := 0, 0
	for i := range m.Files {
		f := &m.Files[i]
		if f.Kind != "translation" || f.Size < minSize {
			continue
		}
		dir := filepath.Join(hist, strings.NewReplacer("/", "__", " ", "_").Replace(f.Path))
		_ = os.MkdirAll(dir, 0o755)
		newB, err := os.ReadFile(abs[f.Path])
		if err != nil {
			continue
		}
		// the copy still in the update folder is the previous release: remember it
		if old, err := os.ReadFile(filepath.Join(out, "files", filepath.FromSlash(f.Path))); err == nil {
			hp := filepath.Join(dir, sha(old)+".gz")
			if _, err := os.Stat(hp); err != nil {
				_ = os.WriteFile(hp, rowpatch.Gzip(old), 0o644)
				_ = os.Chtimes(hp, prevReleased, prevReleased)
			}
		}
		// remember the new one
		hp := filepath.Join(dir, f.SHA256+".gz")
		if _, err := os.Stat(hp); err != nil {
			_ = os.WriteFile(hp, rowpatch.Gzip(newB), 0o644)
		}
		_ = os.Chtimes(hp, time.Now(), time.Now())
		// older versions, newest first
		ents, _ := os.ReadDir(dir)
		type hv struct {
			sha string
			t   time.Time
		}
		var vs []hv
		for _, e := range ents {
			n := strings.TrimSuffix(e.Name(), ".gz")
			if len(n) != 64 || n == f.SHA256 {
				continue
			}
			if fi, err := e.Info(); err == nil {
				vs = append(vs, hv{n, fi.ModTime()})
			}
		}
		sort.Slice(vs, func(a, b int) bool { return vs[a].t.After(vs[b].t) })
		for k, v := range vs { // forget versions that are too old to get a patch
			if k >= keep {
				_ = os.Remove(filepath.Join(dir, v.sha+".gz"))
			}
		}
		if len(vs) > keep {
			vs = vs[:keep]
		}
		fullGz := len(rowpatch.Gzip(newB))
		for _, v := range vs {
			oldB, err := readGz(filepath.Join(dir, v.sha+".gz"))
			if err != nil || sha(oldB) != v.sha {
				continue // damaged history entry: no patch from it
			}
			z := rowpatch.Gzip(rowpatch.Make(oldB, newB))
			if len(z)*2 > fullGz {
				continue // not worth it, players download the whole file
			}
			rel := "patches/" + f.Path + "." + v.sha[:12] + ".rp"
			dst := filepath.Join(out, filepath.FromSlash(rel))
			_ = os.MkdirAll(filepath.Dir(dst), 0o755)
			if err := os.WriteFile(dst, z, 0o644); err != nil {
				die("%v", err)
			}
			f.Patches = append(f.Patches, ManPatch{From: v.sha, Path: rel, Size: int64(len(z)), SHA256: sha(z)})
			made++
		}
		total++
		if len(f.Patches) > 0 {
			small, big := f.Patches[0].Size, f.Patches[0].Size
			for _, pt := range f.Patches {
				if pt.Size < small {
					small = pt.Size
				}
				if pt.Size > big {
					big = pt.Size
				}
			}
			fmt.Printf("Patches for %s: %d (%d KB to %d KB instead of %d KB for the whole file).\n", f.Path, len(f.Patches), small/1024, big/1024, fullGz/1024)
		}
	}
	if total > 0 && made == 0 {
		fmt.Println("note: no patches yet (first release with patches, or the changes are too big to be worth a patch). Players download the whole files this time.")
	}
}

func pack(args []string) {
	fs := flag.NewFlagSet("pack", flag.ExitOnError)
	src := fs.String("src", "", "folder laid out like the program folder")
	url := fs.String("url", "", "update server address players will use, https://.../")
	pub := fs.String("pub", "", "public key text from 'keygen'")
	zf := fs.String("zip", "SWTLUpdater_player.zip", "zip file to create")
	toolsDir := fs.String("tools", "", "optional: your Tools folder; its files are added to the player zip as Tools\\... (they are NOT published online)")
	turl := fs.String("turl", "", "optional: separate address of the translation repo (https://.../)")
	fs.Parse(args)
	if *src == "" || *url == "" || *pub == "" {
		die("need -src, -url and -pub")
	}
	files, abs := scan(*src)
	z, err := os.Create(*zf)
	if err != nil {
		die("%v", err)
	}
	zw := zip.NewWriter(z)
	added := map[string]bool{}
	for _, f := range files {
		added[f.Path] = true
		w, err := zw.Create(f.Path)
		if err != nil {
			die("%v", err)
		}
		in, err := os.Open(abs[f.Path])
		if err != nil {
			die("%v", err)
		}
		io.Copy(w, in)
		in.Close()
	}
	nTools := 0
	if *toolsDir != "" {
		err := filepath.Walk(*toolsDir, func(p string, d os.FileInfo, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel, _ := filepath.Rel(*toolsDir, p)
			name := "Tools/" + filepath.ToSlash(rel)
			if added[name] {
				return nil
			}
			w, err := zw.Create(name)
			if err != nil {
				return err
			}
			in, err := os.Open(p)
			if err != nil {
				return err
			}
			defer in.Close()
			_, err = io.Copy(w, in)
			nTools++
			return err
		})
		if err != nil {
			die("%v", err)
		}
	}
	w, _ := zw.Create("SWTLUpdater.ini")
	fmt.Fprintf(w, "; SW TL Updater settings (the program asks for the rest on first start)\nUpdateURL=%s\nUpdateKey=%s\n", *url, *pub)
	if *turl != "" {
		fmt.Fprintf(w, "TranslationURL=%s\n", *turl)
	}
	fmt.Fprintf(w, "UpdateAtStartup=yes\n")
	zw.Close()
	z.Close()
	fmt.Println("Player zip written:", *zf, "-", len(files), "file(s) +", nTools, "tool file(s) + a settings file with the update address and key.")
}

func verify(args []string) {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	out := fs.String("out", "", "the published folder")
	pubS := fs.String("pub", "", "public key text")
	fs.Parse(args)
	pub, err := base64.StdEncoding.DecodeString(strings.TrimSpace(*pubS))
	if err != nil || len(pub) != ed25519.PublicKeySize {
		die("invalid public key")
	}
	mb, err := os.ReadFile(filepath.Join(*out, "manifest.json"))
	if err != nil {
		die("%v", err)
	}
	sb, _ := os.ReadFile(filepath.Join(*out, "manifest.json.sig"))
	sig, _ := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sb)))
	if !ed25519.Verify(ed25519.PublicKey(pub), mb, sig) {
		die("SIGNATURE INVALID")
	}
	var m Manifest
	if err := json.Unmarshal(mb, &m); err != nil {
		die("%v", err)
	}
	bad := 0
	for _, f := range m.Files {
		b, err := os.ReadFile(filepath.Join(*out, "files", filepath.FromSlash(f.Path)))
		h := sha256.Sum256(b)
		if err != nil || int64(len(b)) != f.Size || hex.EncodeToString(h[:]) != f.SHA256 {
			fmt.Println("MISMATCH:", f.Path)
			bad++
		}
	}
	np := 0
	for _, f := range m.Files {
		for _, pt := range f.Patches {
			b, err := os.ReadFile(filepath.Join(*out, filepath.FromSlash(pt.Path)))
			if err != nil || int64(len(b)) != pt.Size || sha(b) != pt.SHA256 {
				fmt.Println("PATCH MISMATCH:", pt.Path)
				bad++
				continue
			}
			if _, perr := rowpatch.Gunzip(b, 1<<31); perr != nil {
				fmt.Println("PATCH UNREADABLE:", pt.Path)
				bad++
			}
			np++
		}
	}
	if bad > 0 {
		die("%d file(s) do not match the signed list", bad)
	}
	fmt.Printf("OK - signature valid, version %s, %d file(s) match, %d patch(es) match.\n", m.Version, len(m.Files), np)
}
