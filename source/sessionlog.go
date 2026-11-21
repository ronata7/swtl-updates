package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"time"
)

// The session log (logs\session_<date>_<time>.log) is a plain timeline of one run of the program:
// what started, which menu entries were chosen, what was typed at a prompt, which settings changed
// and which warnings or errors came up. Secrets (keys, passwords) are never written to it.

var sess struct {
	mu sync.Mutex
	f  *os.File
}

func openSession(dir string) {
	if os.MkdirAll(dir, 0o755) != nil {
		return
	}
	name := "session_" + time.Now().Format("20060102_150405") + ".log"
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	sess.mu.Lock()
	sess.f = f
	sess.mu.Unlock()
}

func sessf(format string, a ...any) {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	if sess.f != nil {
		fmt.Fprintf(sess.f, "%s %s\n", time.Now().Format("15:04:05"), fmt.Sprintf(format, a...))
	}
}

// secretPrompt reports whether the answer to this prompt must not be logged.
func secretPrompt(msg string) bool {
	m := strings.ToLower(msg)
	return strings.Contains(m, "key") || strings.Contains(m, "password") || strings.Contains(m, "passwort")
}

var secretFields = map[string]bool{"UpdateKey": true, "ArchiveKey": true, "TranslationKey": true}

func fieldText(name string, v reflect.Value) string {
	if secretFields[name] {
		if strings.TrimSpace(v.String()) == "" {
			return "(not set)"
		}
		return "(set)"
	}
	return fmt.Sprintf("%v", v.Interface())
}

// configDump lists every setting for the start of the session log (keys hidden).
func configDump(c *Config) []string {
	var out []string
	v := reflect.ValueOf(*c)
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		out = append(out, fmt.Sprintf("%s = %s", t.Field(i).Name, fieldText(t.Field(i).Name, v.Field(i))))
	}
	return out
}

// configDiff lists the settings that differ ("Name: old -> new"; keys only say "changed").
func configDiff(before, after *Config) []string {
	var out []string
	b, a := reflect.ValueOf(*before), reflect.ValueOf(*after)
	t := b.Type()
	for i := 0; i < t.NumField(); i++ {
		if reflect.DeepEqual(b.Field(i).Interface(), a.Field(i).Interface()) {
			continue
		}
		n := t.Field(i).Name
		if secretFields[n] {
			out = append(out, fmt.Sprintf("%s: changed", n))
		} else {
			out = append(out, fmt.Sprintf("%s: %s -> %s", n, fieldText(n, b.Field(i)), fieldText(n, a.Field(i))))
		}
	}
	return out
}

func copyConfig(c *Config) *Config {
	n := *c
	n.ExcludeTables = append([]string(nil), c.ExcludeTables...)
	return &n
}

var cfgSnap *Config // the settings as last loaded or saved, to log what changed

func sessStart(a *App) {
	sessf("SW TL Updater %s started (%s/%s)", version, runtime.GOOS, runtime.GOARCH)
	sessf("exe: %s", a.self)
	sessf("folder: %s", a.dir)
	sessf("arguments: %v", os.Args[1:])
	sessf("settings file: %s", a.cfgPath)
	for _, l := range configDump(a.cfg) {
		sessf("  setting %s", l)
	}
}

var menuNames = map[string]string{"1": "Patch now", "2": "Dry run", "3": "Settings", "4": "Restore a backup",
	"5": "Open logs folder", "6": "About", "7": "Check for updates", "8": "Make a report", "0": "Exit", "q": "Exit"}
