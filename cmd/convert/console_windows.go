package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var procGetConsoleProcessList = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleProcessList")

// consoleClosesOnExit reports whether this process is the only one attached
// to its console. That happens when Explorer creates a console for a drag and
// drop or double click, and the window closes as soon as the process exits.
// When run from cmd.exe or PowerShell the shell is attached too.
func consoleClosesOnExit() bool {
	if procGetConsoleProcessList.Find() != nil {
		return false
	}
	var processIDs [2]uint32
	count, _, _ := procGetConsoleProcessList.Call(uintptr(unsafe.Pointer(&processIDs[0])), uintptr(len(processIDs)))
	return count == 1
}
