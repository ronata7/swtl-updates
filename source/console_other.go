//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"syscall"
)

func enableVT() {}

func setTitle(t string) {}

var lockFile *os.File

// acquireSingleInstance (non-Windows build, used for testing only) uses a lock file.
func acquireSingleInstance() bool {
	f, err := os.OpenFile(filepath.Join(os.TempDir(), "swtl-updater.lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return true
	}
	if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		f.Close()
		return false
	}
	lockFile = f
	return true
}

func revealFile(path string) {}
