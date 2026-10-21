//go:build !windows

package main

// consoleClosesOnExit reports false outside Windows, where a terminal
// outlives the process it ran.
func consoleClosesOnExit() bool {
	return false
}
