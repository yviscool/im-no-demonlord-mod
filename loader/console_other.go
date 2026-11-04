//go:build !windows

package main

// attachParentConsole is a no-op where programs keep their parent's streams.
func attachParentConsole() {}
