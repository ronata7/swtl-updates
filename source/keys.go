package main

import (
	"fmt"
	"os"
	"path/filepath"

	"swtlupdater/internal/archivekey"
)

// archivePassword finds the password of the game's data file. Sources, first match wins:
//  1. ArchiveKey= in the settings file (a key line pasted by the player)
//  2. key lines (SWTLKEY1:data12:...) in README.txt next to the program
//
// A key line is encrypted text, see internal/archivekey. When the game changes its password a new line is
// added to README.txt (it arrives with the normal signed program update) or pasted into the settings file.
func (a *App) archivePassword(data string) (string, error) {
	v := ""
	if a.cfg != nil {
		v = a.cfg.ArchiveKey
	}
	if v == "" {
		v = readKV(a.cfgPath)["archivekey"]
	}
	if v != "" {
		if pw, ok := archivekey.Find(v, data); ok {
			return pw, nil
		}
	}
	if b, err := os.ReadFile(filepath.Join(a.dir, "README.txt")); err == nil {
		if pw, ok := archivekey.Find(string(b), data); ok {
			return pw, nil
		}
	}
	return "", fmt.Errorf("no archive key for %s", data)
}

func keyHelp(data string) string {
	return "The archive key for " + data + " is missing or not readable.\n" +
		"  The key is a line of text starting with SWTLKEY1: in README.txt (next to the program).\n" +
		"  Keep README.txt as it came, or paste the key line you were sent into SWTLUpdater.ini as:\n" +
		"  ArchiveKey=SWTLKEY1:" + data + ":...\n" +
		"  Questions? Discord: ronata"
}
