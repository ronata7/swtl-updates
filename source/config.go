package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type Config struct {
	SWPath         string
	TLPath         string
	PatcherPath    string
	DataSource     string
	DataOutput     string
	BackupKeep     int
	LogsKeep       int // newest logs / reports / schemas kept by the clean-up (0 = keep all)
	ExcludeTables  []string
	AskAtStartup   bool
	UpdateURL      string
	UpdateKey      string
	TranslationURL string // optional second source that only provides "TL csv/"
	ArchiveKey     string // key line(s) pasted by the player, see archivekey
	TranslationKey string // empty = same key as UpdateKey
	UpdateAtStart  bool
	Converter      string // advanced: builtin (default) | swadmin | check (run both, compare, use swadmin's result)
}

var envRe = regexp.MustCompile(`%([^%]+)%`)

func expand(s, appDir string) string {
	return envRe.ReplaceAllStringFunc(s, func(m string) string {
		k := m[1 : len(m)-1]
		if strings.EqualFold(k, "CurrentDir") {
			return appDir
		}
		if v := os.Getenv(k); v != "" {
			return v
		}
		return m
	})
}

func readKV(path string) map[string]string {
	out := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(sc.Text(), "\ufeff"))
		if line == "" || line[0] == ';' || line[0] == '#' {
			continue
		}
		if i := strings.Index(line, "="); i > 0 {
			out[strings.ToLower(strings.TrimSpace(line[:i]))] = strings.TrimSpace(line[i+1:])
		}
	}
	return out
}

func defaultConfig(appDir string) *Config {
	return &Config{
		SWPath:        `C:\Wemade Online\Soulworker`,
		TLPath:        filepath.Join(appDir, "TL csv"),
		PatcherPath:   filepath.Join(os.Getenv("LocalAppData"), "SWPatcher", "jp", "English", "datas"),
		DataSource:    "data12",
		DataOutput:    "data12",
		BackupKeep:    5,
		LogsKeep:      30,
		ExcludeTables: []string{"tb_Loading_String", "tb_Option_String", "tb_Talk_String"},
		AskAtStartup:  true,
		UpdateAtStart: true,
		Converter:     "builtin",
	}
}

// first non-empty value among the given (lower-case) keys
func pick(kv map[string]string, keys ...string) string {
	for _, k := range keys {
		if v := kv[k]; v != "" {
			return v
		}
	}
	return ""
}

func parseYes(s string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "yes", "y", "true", "1", "on":
		return true
	case "no", "n", "false", "0", "off":
		return false
	}
	return def
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// LoadConfig layers: built-in defaults < SWTLUpdater.ini
// (older beta builds used other key names in the same file; those are still accepted).
func LoadConfig(appDir string) (*Config, string) {
	c := defaultConfig(appDir)
	iniPath := filepath.Join(appDir, "SWTLUpdater.ini")
	set := func(dst *string, v string) {
		if v != "" {
			*dst = expand(v, appDir)
		}
	}
	kv := readKV(iniPath)
	set(&c.SWPath, pick(kv, "gamefolder", "swpath"))
	set(&c.TLPath, pick(kv, "translationfolder", "tlpath"))
	set(&c.PatcherPath, pick(kv, "patcherfolder", "patcherpath"))
	set(&c.DataSource, pick(kv, "sourcedatafile", "datasource"))
	set(&c.DataOutput, pick(kv, "outputdatafile", "dataoutput"))
	if n, err := strconv.Atoi(pick(kv, "backupstokeep", "backupkeep")); err == nil && n >= 0 {
		c.BackupKeep = n
	}
	if n, err := strconv.Atoi(strings.TrimSpace(kv["logstokeep"])); err == nil && n >= 0 {
		c.LogsKeep = n
	}
	if v, ok := kv["skiptables"]; ok {
		c.ExcludeTables = splitList(v)
	} else if v, ok := kv["excludetables"]; ok {
		c.ExcludeTables = splitList(v)
	}
	c.AskAtStartup = parseYes(kv["askatstartup"], true)
	c.UpdateURL = strings.TrimSpace(kv["updateurl"])
	c.UpdateKey = strings.TrimSpace(kv["updatekey"])
	c.TranslationURL = strings.TrimSpace(kv["translationurl"])
	c.TranslationKey = strings.TrimSpace(kv["translationkey"])
	c.ArchiveKey = strings.TrimSpace(kv["archivekey"])
	c.UpdateAtStart = parseYes(kv["updateatstartup"], true)
	switch v := strings.ToLower(strings.TrimSpace(kv["converter"])); v {
	case "builtin", "check", "swadmin":
		c.Converter = v
	}
	if !filepath.IsAbs(c.TLPath) {
		c.TLPath = filepath.Join(appDir, c.TLPath)
	}
	if cfgSnap != nil {
		for _, d := range configDiff(cfgSnap, c) {
			sessf("settings file was changed outside the program: %s", d)
		}
	}
	cfgSnap = copyConfig(c)
	return c, iniPath
}

func splitList(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (c *Config) Save(path string) error {
	if cfgSnap != nil {
		for _, d := range configDiff(cfgSnap, c) {
			sessf("setting changed: %s", d)
		}
	}
	cfgSnap = copyConfig(c)
	var b strings.Builder
	b.WriteString("; SW TL Updater settings.\n; Edit this file with Notepad, or use \"Settings\" in the program menu.\n; Lines starting with ; are comments.\n\n")
	b.WriteString("; Folder where SoulWorker is installed (the one containing SoulWorker100.exe)\n")
	fmt.Fprintf(&b, "GameFolder=%s\n\n", c.SWPath)
	b.WriteString("; Folder with your translated .csv files\n")
	fmt.Fprintf(&b, "TranslationFolder=%s\n\n", c.TLPath)
	b.WriteString("; SWPatcher's \"datas\" folder - the patched file is copied here too; if the folder does not exist the copy is skipped\n")
	fmt.Fprintf(&b, "PatcherFolder=%s\n\n", c.PatcherPath)
	b.WriteString("; Name of the game data file to patch, without .v (found in the game's datas folder)\n")
	fmt.Fprintf(&b, "SourceDataFile=%s\n\n", c.DataSource)
	b.WriteString("; Name for the patched file, without .v\n")
	fmt.Fprintf(&b, "OutputDataFile=%s\n\n", c.DataOutput)
	b.WriteString("; How many older versions to keep in the backup folder (0 = keep all)\n")
	fmt.Fprintf(&b, "BackupsToKeep=%d\n\n", c.BackupKeep)
	b.WriteString("; How many of the newest log files to keep (reports and schemas keep fewer; 0 = keep all)\n")
	fmt.Fprintf(&b, "LogsToKeep=%d\n\n", c.LogsKeep)
	b.WriteString("; Tables that are never patched, separated by commas\n")
	fmt.Fprintf(&b, "SkipTables=%s\n\n", strings.Join(c.ExcludeTables, ","))
	b.WriteString("; Show the settings at startup and ask if you want to change them (yes / no)\n")
	fmt.Fprintf(&b, "AskAtStartup=%s\n\n", yesNo(c.AskAtStartup))
	b.WriteString("; Online updates: web address of the update server (must start with https://) and its public key.\n")
	b.WriteString("; Both come with your download. Leave them empty to turn online updates off.\n")
	fmt.Fprintf(&b, "UpdateURL=%s\n", c.UpdateURL)
	fmt.Fprintf(&b, "UpdateKey=%s\n", c.UpdateKey)
	b.WriteString("; Optional: the translation files come from a separate address (same key unless TranslationKey is set).\n")
	fmt.Fprintf(&b, "TranslationURL=%s\n", c.TranslationURL)
	fmt.Fprintf(&b, "TranslationKey=%s\n\n", c.TranslationKey)
	b.WriteString("; Look for updates when the program starts (yes / no)\n")
	fmt.Fprintf(&b, "UpdateAtStartup=%s\n", yesNo(c.UpdateAtStart))
	b.WriteString("; Advanced: how the schema and the game tables are made - builtin (built in, default), swadmin (the SWAdmin tool),\n")
	b.WriteString("; or check (do both, compare them and report differences; the SWAdmin result is used).\n")
	fmt.Fprintf(&b, "Converter=%s\n", c.Converter)
	if c.ArchiveKey != "" {
		b.WriteString("; Archive key (encrypted text, do not change unless you were sent a new one)\n")
		fmt.Fprintf(&b, "ArchiveKey=%s\n", c.ArchiveKey)
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// Validate returns a problem description for the first thing wrong, or nil.
func (c *Config) Validate() error {
	if !exists(filepath.Join(c.SWPath, "SoulWorker100.exe")) && !exists(filepath.Join(c.SWPath, "SoulWorker.exe")) {
		return fmt.Errorf("SoulWorker not found in: %s", c.SWPath)
	}
	if !exists(filepath.Join(c.SWPath, "SoulWorker64.dll")) {
		return fmt.Errorf("SoulWorker64.dll missing in: %s", c.SWPath)
	}
	if !exists(filepath.Join(c.SWPath, "datas", c.DataSource+".v")) {
		return fmt.Errorf("%s.v not found in %s", c.DataSource, filepath.Join(c.SWPath, "datas"))
	}
	if !isDir(c.TLPath) {
		return fmt.Errorf("translation CSV folder not found: %s", c.TLPath)
	}
	return nil
}
