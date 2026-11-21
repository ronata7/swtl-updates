package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var version = "2.24.1" // may be overridden at build time with -ldflags "-X main.version=..."

func appTitle() string { return "SW TL Updater " + version + " - by ronata" }

var stdin = bufio.NewReader(os.Stdin)

func prompt(msg string) string {
	fmt.Print(msg)
	s, err := stdin.ReadString('\n')
	if err != nil && s == "" {
		sessf("input closed - exiting")
		os.Exit(0) // input closed (window closed / piped input ended)
	}
	ans := strings.TrimSpace(s)
	if secretPrompt(msg) {
		sessf("input: %q -> (hidden)", strings.TrimSpace(msg))
	} else {
		sessf("input: %q -> %q", strings.TrimSpace(msg), ans)
	}
	return ans
}

func main() {
	enableVT()
	setTitle(appTitle())
	var (
		dry      = flag.Bool("dry-run", false, "run everything but don't write any result files")
		run      = flag.Bool("run", false, "patch now (no menu)")
		restore  = flag.Bool("restore", false, "restore the newest backup into the patcher folder")
		keep     = flag.Bool("keep-work", false, "keep the temporary work folder")
		strict   = flag.Bool("strict", false, "treat CSV problems as errors")
		appdir   = flag.String("appdir", "", "folder holding Tools, TL csv, settings (default: next to the exe)")
		cfgSetup = flag.Bool("settings", false, "show the settings menu then exit")
		doUpdate = flag.Bool("update", false, "check the update server and install updates first")
		report   = flag.Bool("report", false, "make a report file (logs, settings, versions) to send to the author")
		expRef   = flag.Bool("export-ref", false, "export the string/script tables, schema and version of a game install (for the translation team)")
		expGame  = flag.String("game", "", "with --export-ref: the game folder (with SoulWorker64.dll)")
		expReg   = flag.String("region", "", "with --export-ref: jp, kr or gb")
		expData  = flag.String("data", "", "with --export-ref: data file name (default data12, data11 for gb)")
		expPw    = flag.String("pwfile", "", "with --export-ref: text file with the archive password (jp has a built-in one)")
		expOut   = flag.String("out", "", "with --export-ref: output folder (default <app>\\reference)")
	)
	flag.Parse()

	cli := false // any switch other than --appdir means command-line mode (no menu)
	flag.Visit(func(f *flag.Flag) {
		if f.Name != "appdir" {
			cli = true
		}
	})

	onlyUpdate := *doUpdate // --update on its own: update and stop (no menu, no patching)
	flag.Visit(func(f *flag.Flag) {
		if f.Name != "update" && f.Name != "appdir" {
			onlyUpdate = false
		}
	})
	relaunched := os.Getenv("SWTL_RELAUNCH") == "1"
	if exe, err := os.Executable(); err == nil {
		_ = os.Remove(exe + ".old") // left over from a self-update
	}
	got := acquireSingleInstance()
	for i := 0; !got && relaunched && i < 50; i++ { // the old copy may still be shutting down
		time.Sleep(100 * time.Millisecond)
		got = acquireSingleInstance()
	}
	if !got {
		fmt.Println("\n SW TL Updater is already running in another window.")
		fmt.Println(" Close the other one first - two copies must not patch at the same time.")
		if !cli {
			prompt("\n Press Enter to exit...")
		}
		os.Exit(1)
	}

	dir := *appdir
	if dir == "" {
		exe, _ := os.Executable()
		dir = filepath.Dir(exe)
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	cfg, cfgPath := LoadConfig(dir)
	self, _ := os.Executable()
	app := &App{dir: dir, cfg: cfg, cfgPath: cfgPath, self: self}
	openSession(app.logsDir())
	sessStart(app)

	if !isDir(filepath.Join(dir, "Tools")) {
		// first start of a bare download: fetch the program files from the update server
		if strings.TrimSpace(cfg.UpdateURL) != "" && strings.TrimSpace(cfg.UpdateKey) != "" {
			fmt.Println("\n First start - downloading the program files...")
			restarted, err := app.Update(false, false, os.Args[1:], !onlyUpdate)
			if err != nil {
				fmt.Println("  x", err)
			}
			if restarted {
				os.Exit(0)
			}
		}
		if !isDir(filepath.Join(dir, "Tools")) {
			fmt.Printf("\n The 'Tools' folder is missing next to the updater (%s).\n", dir)
			fmt.Println(toolsHelp)
			if !cli {
				prompt("Press Enter to exit...")
			}
			os.Exit(1)
		}
	}

	if cli { // command-line mode
		if *doUpdate {
			restarted, err := app.Update(false, false, os.Args[1:], !onlyUpdate)
			if err != nil {
				fmt.Println("  x", err)
				os.Exit(1)
			}
			if restarted || onlyUpdate {
				os.Exit(0)
			}
		}
		switch {
		case *cfgSetup:
			app.Settings()
			return
		case *restore:
			os.Exit(app.Restore(false))
		case *report:
			os.Exit(app.MakeReport())
		case *expRef:
			os.Exit(app.ExportRef(ExportOpts{Game: *expGame, Region: *expReg, Data: *expData, PwFile: *expPw, Out: *expOut, Keep: *keep}))
		default:
			_ = run
			if err := app.ensureConfig(false); err != nil {
				fmt.Println("  x", err)
				os.Exit(1)
			}
			os.Exit(app.RunPatch(RunOpts{Dry: *dry, KeepWork: *keep, Strict: *strict}))
		}
	}

	// interactive menu (double-click)
	if relaunched {
		fmt.Printf("\n Updated - now running version %s.\n", version)
	} else {
		if app.cfg.UpdateAtStart && strings.TrimSpace(app.cfg.UpdateURL) != "" {
			restarted, err := app.Update(true, true, os.Args[1:], true)
			if err != nil {
				fmt.Println("  x Update:", err)
			}
			if restarted {
				os.Exit(0)
			}
		}
		app.cleanOld()
		app.StartupReview(!exists(cfgPath) || readKV(cfgPath)["gamefolder"] == "")
		if msg := app.gameChangedNotice(); msg != "" {
			fmt.Printf("\n ! %s\n", msg)
		}
	}
	for {
		fmt.Printf("\n SW TL Updater %s  -  by ronata\n -----------------------------\n", version)
		fmt.Println("  1) Patch now")
		fmt.Println("  2) Dry run (check everything, write nothing)")
		fmt.Println("  3) Settings")
		fmt.Println("  4) Restore a backup")
		fmt.Println("  5) Open logs folder")
		fmt.Println("  6) About, credits & help")
		fmt.Println("  7) Check for updates")
		fmt.Println("  8) Make a report for ronata (logs, versions)")
		fmt.Println("  0) Exit")
		choice := prompt(" > ")
		if n, ok := menuNames[choice]; ok {
			sessf("menu: %s", n)
		} else {
			sessf("menu: unknown choice %q", choice)
		}
		switch choice {
		case "1", "2":
			// re-read settings in case ini was edited
			app.cfg, _ = LoadConfig(dir)
			if err := app.ensureConfig(true); err != nil {
				fmt.Println("  x", err)
				continue
			}
			dryRun := choice == "2"
			fmt.Println()
			code := app.RunPatch(RunOpts{Dry: dryRun, Interactive: true, Strict: *strict, KeepWork: *keep})
			sessf("patch finished, result code %d", code)
		case "3":
			app.Settings()
		case "4":
			app.Restore(true)
		case "5":
			_ = os.MkdirAll(app.logsDir(), 0o755)
			if runtime.GOOS == "windows" {
				_ = exec.Command("explorer", app.logsDir()).Start()
			} else {
				fmt.Println(app.logsDir())
			}
		case "6":
			app.About()
		case "7":
			app.cfg, _ = LoadConfig(dir)
			restarted, err := app.Update(true, false, os.Args[1:], true)
			if err != nil {
				fmt.Println("  x", err)
			}
			if restarted {
				os.Exit(0)
			}
		case "8":
			app.cfg, _ = LoadConfig(dir)
			app.MakeReport()
		case "0", "q":
			sessf("exit")
			return
		}
	}
}
