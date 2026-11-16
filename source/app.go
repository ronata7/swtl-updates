package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type App struct {
	self     string // path of the running exe, captured before any self-update
	selfDone bool   // the exe was already replaced during this run
	dir      string
	cfgPath  string
	cfg      *Config
}

func (a *App) logsDir() string   { return filepath.Join(a.dir, "logs") }
func (a *App) backupDir() string { return filepath.Join(a.dir, "backup") }

func (a *App) listBackups() []string {
	ents, _ := os.ReadDir(a.backupDir())
	var out []string
	pre := a.cfg.DataOutput + "_"
	for _, e := range ents {
		if !e.IsDir() && strings.HasPrefix(e.Name(), pre) && strings.HasSuffix(e.Name(), ".v") {
			out = append(out, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(out)))
	return out
}

func (a *App) pruneBackups(log *Logger) {
	if a.cfg.BackupKeep <= 0 {
		return
	}
	b := a.listBackups()
	for i := a.cfg.BackupKeep; i < len(b); i++ {
		_ = os.Remove(filepath.Join(a.backupDir(), b[i]))
	}
}

// guardProcesses warns when the game or SWPatcher is running. It returns false if the
// user (interactive mode) decides to cancel.
func (a *App) guardProcesses(log *Logger, interactive bool) bool {
	bad := false
	if processRunning("SoulWorker100.exe") || processRunning("SoulWorker.exe") {
		log.Warn("SoulWorker is running - close the game first, otherwise it may be using the files being replaced")
		bad = true
	}
	if processRunning("SWPatcher.exe") {
		log.Warn("SWPatcher is running - make sure it is idle (not waiting for the client)")
		bad = true
	}
	if bad && interactive && !confirm("  Continue anyway?") {
		return false
	}
	return true
}

func (a *App) Restore(interactive bool) int {
	log := &Logger{}
	b := a.listBackups()
	if len(b) == 0 {
		log.Fail("No backups found in %s", a.backupDir())
		return 1
	}
	pick := b[0]
	if interactive {
		for i, n := range b {
			fmt.Printf("  %d) %s\n", i+1, n)
		}
		s := prompt("Restore which one? (number, Enter = cancel): ")
		var i int
		if _, err := fmt.Sscanf(s, "%d", &i); err != nil || i < 1 || i > len(b) {
			return 0
		}
		pick = b[i-1]
	}
	if !isDir(a.cfg.PatcherPath) {
		log.Fail("Patcher folder not found: %s", a.cfg.PatcherPath)
		return 1
	}
	if !a.guardProcesses(log, interactive) {
		return 0
	}
	dst := filepath.Join(a.cfg.PatcherPath, a.cfg.DataOutput+".v")
	if err := atomicCopy(filepath.Join(a.backupDir(), pick), dst); err != nil {
		log.Fail("%v", err)
		return 1
	}
	log.OK("Restored %s -> %s", pick, dst)
	return 0
}

// ---- settings -----------------------------------------------------------

func browseFolder(title string) string {
	if runtime.GOOS != "windows" {
		return ""
	}
	ps := fmt.Sprintf("(new-object -COM 'Shell.Application').BrowseForFolder(0,'%s',0,'::').self.path", title)
	out, err := exec.Command("powershell", "-NoProfile", "-Command", ps).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func askPath(label, cur, dialogTitle string) string {
	fmt.Printf("\n %s\n   now: %s\n", label, cur)
	s := prompt("   Type a new path, press Enter to browse for a folder, or type - to keep it: ")
	switch s {
	case "-":
		return cur
	case "":
		if p := browseFolder(dialogTitle); p != "" {
			return p
		}
		fmt.Println("   (no folder chosen - kept as is)")
		return cur
	}
	return strings.Trim(s, `"`)
}

func (a *App) editPaths(onlyBad bool) {
	c := a.cfg
	if !onlyBad || !a.gameOK() {
		c.SWPath = askPath("Game folder - where SoulWorker is installed", c.SWPath, "Select Soulworker Game Folder!")
	}
	if !onlyBad || !a.tlOK() {
		c.TLPath = askPath("Translation folder - your translated .csv files", c.TLPath, "Select Translation .csv Folder!")
	}
	_ = c.Save(a.cfgPath)
}

func (a *App) gameOK() bool {
	c := a.cfg
	return exists(filepath.Join(c.SWPath, "SoulWorker64.dll")) && exists(filepath.Join(c.SWPath, "datas", c.DataSource+".v"))
}

func (a *App) tlOK() bool {
	m, _ := filepath.Glob(filepath.Join(a.cfg.TLPath, "*.csv"))
	return len(m) > 0
}

func okMark(b bool) string {
	if b {
		return "ok"
	}
	return "NOT FOUND"
}

func (a *App) printSettings() {
	c := a.cfg
	fmt.Println()
	fmt.Println(" Current settings")
	fmt.Println(" ----------------")
	fmt.Printf("  Game folder          : %s   [%s]\n", c.SWPath, okMark(a.gameOK()))
	fmt.Printf("  Translation folder   : %s   [%s]\n", c.TLPath, okMark(a.tlOK()))
	fmt.Printf("  Patcher folder       : %s   [%s]\n", c.PatcherPath, okMark(isDir(c.PatcherPath)))
	fmt.Printf("  Game data file       : %s.v  (patched copy is saved as %s.v)\n", c.DataSource, c.DataOutput)
	fmt.Printf("  Backups to keep      : %d\n", c.BackupKeep)
	fmt.Printf("  Tables to skip       : %s\n", strings.Join(c.ExcludeTables, ", "))
	fmt.Printf("  Ask at startup       : %s\n", yesNo(c.AskAtStartup))
	if strings.TrimSpace(c.UpdateURL) == "" {
		fmt.Println("  Online updates       : off (no update address set)")
	} else {
		fmt.Printf("  Online updates       : %s   (check at startup: %s)\n", c.UpdateURL, yesNo(c.UpdateAtStart))
		if strings.TrimSpace(c.TranslationURL) != "" {
			fmt.Printf("  Translation updates  : %s\n", c.TranslationURL)
		}
	}
}

// StartupReview shows the settings and lets the user change them before the menu.
func (a *App) StartupReview(firstRun bool) {
	if !firstRun && !a.cfg.AskAtStartup {
		return
	}
	if firstRun {
		fmt.Println("\n Welcome! This looks like the first run, so here are the starting values.")
	}
	a.printSettings()
	if !a.gameOK() || !a.tlOK() {
		fmt.Println("\n ! Something above says NOT FOUND - type C to fix it.")
	}
	if strings.HasPrefix(strings.ToLower(prompt("\n Press Enter to continue, or type C to change settings: ")), "c") {
		a.Settings()
	}
	if firstRun {
		_ = a.cfg.Save(a.cfgPath)
		fmt.Printf(" Settings saved to %s\n", a.cfgPath)
	}
}

func (a *App) Settings() {
	for {
		c := a.cfg
		fmt.Printf("\n Settings   (saved in %s)\n", filepath.Base(a.cfgPath))
		fmt.Printf("  1) Game folder .............. %s\n", c.SWPath)
		fmt.Printf("  2) Translation folder ....... %s\n", c.TLPath)
		fmt.Printf("  3) Patcher folder ........... %s\n", c.PatcherPath)
		fmt.Printf("  4) Game data file to patch .. %s.v\n", c.DataSource)
		fmt.Printf("  5) Name of the patched file . %s.v\n", c.DataOutput)
		fmt.Printf("  6) Backups to keep .......... %d  (0 = keep all)\n", c.BackupKeep)
		fmt.Printf("  7) Tables to skip ........... %s\n", strings.Join(c.ExcludeTables, ", "))
		fmt.Printf("  8) Ask about settings at startup: %s\n", yesNo(c.AskAtStartup))
		if strings.TrimSpace(c.UpdateURL) == "" {
			fmt.Println("  9) Update address ............ (not set - online updates off)")
		} else {
			fmt.Printf("  9) Update address ............ %s\n", c.UpdateURL)
		}
		fmt.Printf(" 10) Update key ................ %s\n", shortKey(c.UpdateKey))
		fmt.Printf(" 11) Check for updates at startup: %s\n", yesNo(c.UpdateAtStart))
		fmt.Println(" 12) Reset everything to the default values")
		fmt.Println("  0) Back")
		switch prompt(" > ") {
		case "1":
			c.SWPath = askPath("Game folder - where SoulWorker is installed", c.SWPath, "Select Soulworker Game Folder!")
		case "2":
			c.TLPath = askPath("Translation folder - your translated .csv files", c.TLPath, "Select Translation .csv Folder!")
		case "3":
			c.PatcherPath = askPath("Patcher folder - SWPatcher's \"datas\" folder", c.PatcherPath, "Select SWPatcher datas folder")
		case "4":
			if v := prompt("  Game data file name without .v (Enter = keep): "); v != "" {
				c.DataSource = strings.TrimSuffix(v, ".v")
			}
		case "5":
			if v := prompt("  Name for the patched file without .v (Enter = keep): "); v != "" {
				c.DataOutput = strings.TrimSuffix(v, ".v")
			}
		case "6":
			var n int
			if _, err := fmt.Sscanf(prompt("  How many backups to keep (0 = all): "), "%d", &n); err == nil && n >= 0 {
				c.BackupKeep = n
			}
		case "7":
			fmt.Println("  Tables listed here are never patched, e.g. tb_Loading_String")
			if v := prompt("  Names separated by commas, 'none' for no skipped tables (Enter = keep): "); v != "" {
				if strings.EqualFold(v, "none") {
					c.ExcludeTables = nil
				} else {
					c.ExcludeTables = splitList(v)
				}
			}
		case "8":
			c.AskAtStartup = !c.AskAtStartup
			if !c.AskAtStartup {
				fmt.Println("  OK - the settings will no longer be shown at startup (you can turn this back on here).")
			}
		case "9":
			if v := prompt("  Update address, https://... (Enter = keep, - = turn updates off): "); v == "-" {
				c.UpdateURL, c.TranslationURL = "", ""
			} else if v != "" {
				c.UpdateURL = v
			}
		case "10":
			if v := prompt("  Update key (paste it, Enter = keep): "); v != "" {
				c.UpdateKey = v
			}
		case "11":
			c.UpdateAtStart = !c.UpdateAtStart
		case "12":
			if confirm("  Reset all settings to the default values? (the update address and key are kept)") {
				ask, u, k, tu, tk := c.AskAtStartup, c.UpdateURL, c.UpdateKey, c.TranslationURL, c.TranslationKey
				*c = *defaultConfig(a.dir)
				c.AskAtStartup, c.UpdateURL, c.UpdateKey, c.TranslationURL, c.TranslationKey = ask, u, k, tu, tk
			}
		case "0", "":
			_ = c.Save(a.cfgPath)
			return
		}
		_ = c.Save(a.cfgPath)
	}
}

// ensureConfig makes sure the settings are usable; interactively it offers to fix them.
func (a *App) ensureConfig(interactive bool) error {
	for i := 0; i < 5; i++ {
		err := a.cfg.Validate()
		if err == nil {
			return nil
		}
		if !interactive {
			return err
		}
		fmt.Printf("\n  ! %v\n", err)
		a.editPaths(true)
	}
	return a.cfg.Validate()
}

func confirm(q string) bool {
	s := strings.ToLower(prompt(q + " [y/N] "))
	return s == "y" || s == "yes"
}

// About shows credits and where to get help.
func (a *App) About() {
	fmt.Printf("\n %s\n ------------------------------------------------------------\n", appTitle())
	fmt.Println("  Questions or problems?  Discord: ronata   (please attach the newest file from the logs folder)")
	fmt.Println()
	fmt.Println("  Credits - the tools in the Tools folder are used unmodified:")
	fmt.Println("    SWAdmin tools (schema+table steps now built in, first done with them) .. rockon")
	fmt.Println("    SW Res structs dumper ...... Asaduji  -  github.com/Asaduji/SW-Res-structs-dumper")
	fmt.Println("    7-Zip (7z.exe, 7z.dll) ..... Igor Pavlov  -  7-zip.org")
	fmt.Println("    Xor step (built in now) .... first done with Xor by Luigi Auriemma  -  aluigi.org")
	fmt.Println("    Libraries bundled in them .. Newtonsoft.Json (James Newton-King), Zydis (zyantific)")
	fmt.Println("    Updater .................... ronata")
	fmt.Println()
	fmt.Println("  Unofficial fan project, not affiliated with the game's publishers. The game's terms restrict")
	fmt.Println("  modifying game data: use at your own risk, your account could be suspended.")
	prompt("\n Press Enter to go back... ")
}

func shortKey(k string) string {
	k = strings.TrimSpace(k)
	if k == "" {
		return "(not set)"
	}
	if len(k) > 14 {
		return k[:10] + "..."
	}
	return k
}

// toolsHelp explains where the (not redistributed) tools come from.
const toolsHelp = `
 The Tools folder is missing. These programs belong to their authors, so they only come in the player zip
 (not from online updates). Take the Tools folder from that zip, or put these into a folder named "Tools" next to the updater:
    Tools\SW Res structs dumper.exe, .pdb and Zydis.dll                     - SW Res structs dumper by Asaduji
    Tools\7z.exe and 7z.dll                                                - 7-Zip, from 7-zip.org
 (The xor, schema and table steps are built in. Only if you set Converter=swadmin in the .ini do you also need
  Tools\SWAdmin\SWAdmin.Cli.exe, SWAdmin.Core.dll and Newtonsoft.Json.dll - SWAdmin tools by rockon.)
 Questions? Discord: ronata`
