//go:build !windows

package ui

import "fmt"

func runExternal(command, dir string, handle uint32, quiet bool) error {
	return fmt.Errorf("downloads run on Windows")
}

func runInherited(command, dir string, handle uint32) error {
	return fmt.Errorf("the editor runs on Windows")
}
