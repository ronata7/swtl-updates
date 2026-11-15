package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func isDir(p string) bool { st, err := os.Stat(p); return err == nil && st.IsDir() }

func fileSize(p string) (int64, error) {
	st, err := os.Stat(p)
	if err != nil {
		return 0, err
	}
	return st.Size(), nil
}

func hashFile(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	mode := os.FileMode(0o644)
	if st, err := in.Stat(); err == nil {
		mode = st.Mode().Perm()
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// atomicCopy copies via a temp file and renames, so a half-written file is never
// left where the patcher/game could read it. The result is size-checked.
func atomicCopy(src, dst string) error {
	tmp := dst + ".tmp"
	if err := copyFile(src, tmp); err != nil {
		os.Remove(tmp)
		return err
	}
	a, _ := fileSize(src)
	b, _ := fileSize(tmp)
	if a != b {
		os.Remove(tmp)
		return fmt.Errorf("size mismatch copying to %s (%d vs %d bytes)", dst, a, b)
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func tailLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// processRunning reports whether an executable with this name is running (Windows only).
// SWTL_FAKE_PROCS lets the test-suite simulate running programs.
func processRunning(name string) bool {
	ln := strings.ToLower(name)
	if fake := os.Getenv("SWTL_FAKE_PROCS"); fake != "" {
		for _, f := range strings.Split(strings.ToLower(fake), ",") {
			if strings.TrimSpace(f) == ln {
				return true
			}
		}
		return false
	}
	if runtime.GOOS != "windows" {
		return false
	}
	out, err := exec.Command("tasklist", "/FI", "IMAGENAME eq "+name, "/NH", "/FO", "CSV").Output()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(out)), `"`+ln+`"`)
}
