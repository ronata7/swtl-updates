package main

import (
	"fmt"
	"os"
	"sync"
	"time"
)

var useColor = true

const (
	cRed    = "\x1b[91m"
	cGreen  = "\x1b[92m"
	cYellow = "\x1b[93m"
	cCyan   = "\x1b[96m"
	cDim    = "\x1b[90m"
	cReset  = "\x1b[0m"
)

type Logger struct {
	mu    sync.Mutex
	f     *os.File
	Warns []string
	// Downloaded counts the bytes fetched by an online update (patches and whole files).
	Downloaded int64
}

func (l *Logger) file(s string) {
	if l.f != nil {
		fmt.Fprintf(l.f, "%s %s\n", time.Now().Format("15:04:05"), s)
	}
}

func (l *Logger) emit(color, tag, msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if useColor && color != "" {
		fmt.Printf("%s%s%s%s\n", color, tag, msg, cReset)
	} else {
		fmt.Printf("%s%s\n", tag, msg)
	}
	l.file(tag + msg)
}

func (l *Logger) Info(f string, a ...any) { l.emit("", "    ", fmt.Sprintf(f, a...)) }
func (l *Logger) OK(f string, a ...any)   { l.emit(cGreen, "  + ", fmt.Sprintf(f, a...)) }
func (l *Logger) Fail(f string, a ...any) { l.emit(cRed, "  x ", fmt.Sprintf(f, a...)) }
func (l *Logger) Warn(f string, a ...any) {
	m := fmt.Sprintf(f, a...)
	l.Warns = append(l.Warns, m)
	l.emit(cYellow, "  ! ", m)
}
func (l *Logger) Step(i, n int, f string, a ...any) {
	setTitle(fmt.Sprintf("SW TL Updater - step %d/%d: %s", i, n, fmt.Sprintf(f, a...)))
	l.emit(cCyan, fmt.Sprintf("[%d/%d] ", i, n), fmt.Sprintf(f, a...))
}

// Raw writes to the log file only (tool output, command lines).
func (l *Logger) Raw(s string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.file(s)
}
