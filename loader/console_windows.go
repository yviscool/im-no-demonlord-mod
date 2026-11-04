//go:build windows

package main

import (
	"os"
	"syscall"
)

const attachParentProcess = ^uintptr(0) // ATTACH_PARENT_PROCESS (-1)

var procAttachConsole = syscall.NewLazyDLL("kernel32.dll").NewProc("AttachConsole")

// attachParentConsole reconnects the standard streams to the console of the
// shell that started this GUI-subsystem program, so that `status`, `install`,
// `uninstall` and `inject` still print where the user ran them. It does
// nothing when the shell already passed usable handles (a console or a
// redirection), and when the program was started from Explorer, which has no
// console at all.
func attachParentConsole() {
	if stdoutUsable() {
		return
	}
	if r, _, _ := procAttachConsole.Call(attachParentProcess); r == 0 {
		return
	}
	if f, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
		os.Stdout = f
		os.Stderr = f
	}
	if f, err := os.OpenFile("CONIN$", os.O_RDONLY, 0); err == nil {
		os.Stdin = f
	}
}

// stdoutUsable reports whether the process inherited a handle it can write to,
// as it does from a console or a pipe.
func stdoutUsable() bool {
	h, err := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	if err != nil || h == 0 || h == syscall.InvalidHandle {
		return false
	}
	t, err := syscall.GetFileType(h)
	return err == nil && t != syscall.FILE_TYPE_UNKNOWN
}
