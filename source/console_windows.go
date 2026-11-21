//go:build windows

package main

import (
	"os/exec"
	"syscall"
	"unsafe"
)

// enableVT turns on ANSI colours and UTF-8 output in the Windows console.
func enableVT() {
	k := syscall.NewLazyDLL("kernel32.dll")
	getMode := k.NewProc("GetConsoleMode")
	setMode := k.NewProc("SetConsoleMode")
	k.NewProc("SetConsoleOutputCP").Call(65001)
	h, err := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	if err != nil {
		useColor = false
		return
	}
	var m uint32
	if r, _, _ := getMode.Call(uintptr(h), uintptr(unsafe.Pointer(&m))); r == 0 {
		useColor = false // output is redirected
		return
	}
	if r, _, _ := setMode.Call(uintptr(h), uintptr(m|0x0004)); r == 0 {
		useColor = false
	}
}

// setTitle changes the console / terminal tab title.
func setTitle(t string) {
	p, err := syscall.UTF16PtrFromString(t)
	if err != nil {
		return
	}
	syscall.NewLazyDLL("kernel32.dll").NewProc("SetConsoleTitleW").Call(uintptr(unsafe.Pointer(p)))
}

// acquireSingleInstance returns false if another copy of the updater is already running.
// The mutex handle is deliberately never closed; Windows releases it when the process exits.
func acquireSingleInstance() bool {
	name, err := syscall.UTF16PtrFromString("Local\\SWTLUpdater-ronata-single-instance")
	if err != nil {
		return true
	}
	h, _, callErr := syscall.NewLazyDLL("kernel32.dll").NewProc("CreateMutexW").Call(0, 0, uintptr(unsafe.Pointer(name)))
	if h == 0 {
		return true // could not create it - do not block the user
	}
	if errno, ok := callErr.(syscall.Errno); ok && errno == 183 { // ERROR_ALREADY_EXISTS
		return false
	}
	return true
}

// revealFile opens Explorer with the file selected. The argument is passed exactly as
// explorer /select,"path" - Go would otherwise wrap the whole /select,... text in quotes when the
// path has spaces, and Explorer then opens the Documents folder instead.
func revealFile(path string) {
	c := exec.Command("explorer.exe")
	c.SysProcAttr = &syscall.SysProcAttr{CmdLine: `explorer.exe /select,"` + path + `"`}
	_ = c.Start()
}
