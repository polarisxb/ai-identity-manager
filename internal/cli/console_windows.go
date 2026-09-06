//go:build windows

package cli

import "syscall"

func MaybeHideConsole(args []string) {
	if !wantsHiddenConsole(args) {
		return
	}
	hideConsoleWindow()
}

func hideConsoleWindow() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	user32 := syscall.NewLazyDLL("user32.dll")
	hwnd, _, _ := kernel32.NewProc("GetConsoleWindow").Call()
	if hwnd == 0 {
		return
	}
	const swHide = 0
	user32.NewProc("ShowWindow").Call(hwnd, swHide)
}
