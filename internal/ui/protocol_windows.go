//go:build windows

package ui

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

// The external program is started with CreateProcessW directly.
// A 32-bit or 16-bit executable both start this way.
// The caller socket is duplicated for the child. The child can close
// that copy. This process keeps the original, so the connection stays up.

var (
	procCreateProcessW       = syscall.NewLazyDLL("kernel32.dll").NewProc("CreateProcessW")
	procDuplicateHandle      = syscall.NewLazyDLL("kernel32.dll").NewProc("DuplicateHandle")
	procGetCurrentProcess    = syscall.NewLazyDLL("kernel32.dll").NewProc("GetCurrentProcess")
	procSetHandleInformation = syscall.NewLazyDLL("kernel32.dll").NewProc("SetHandleInformation")
	procGetStdHandle         = syscall.NewLazyDLL("kernel32.dll").NewProc("GetStdHandle")
	procSetSockOpt           = syscall.NewLazyDLL("ws2_32.dll").NewProc("setsockopt")
	procIoctlsocket          = syscall.NewLazyDLL("ws2_32.dll").NewProc("ioctlsocket")
)

const (
	handleFlagInherit   = 0x00000001
	duplicateSameAccess = 0x00000002
	createNoWindow      = 0x08000000
)

func runExternal(command, dir string, handle uint32, quiet bool) error {
	command = trimSpace(command)
	if command == "" {
		return fmt.Errorf("empty protocol command")
	}
	var sock syscall.Handle
	if handle != 0 {
		dup, err := dupSocket(syscall.Handle(handle))
		if err != nil {
			return err
		}
		defer syscall.CloseHandle(dup)
		sock = dup
		command = strings.ReplaceAll(command, "@@HANDLE@@", itoa(uint32(dup)))
	} else {
		command = strings.ReplaceAll(command, "@@HANDLE@@", "0")
	}

	cmd := make([]uint16, len(command)+32)
	copy(cmd, syscall.StringToUTF16(command))
	var dirp *uint16
	if dir != "" {
		p, err := syscall.UTF16PtrFromString(dir)
		if err != nil {
			return err
		}
		dirp = p
	}

	var flags uint32
	var si syscall.StartupInfo
	si.Cb = uint32(unsafe.Sizeof(si))
	si.Flags = syscall.STARTF_USESTDHANDLES
	if sock != 0 {
		if quiet {
			flags = createNoWindow
		}
		si.StdInput = sock
		si.StdOutput = sock
		si.StdErr = sock
	} else {
		si.StdInput = stdHandle(0xFFFFFFF6)
		si.StdOutput = stdHandle(0xFFFFFFF5)
		si.StdErr = stdHandle(0xFFFFFFF4)
		setInherit(si.StdInput, true)
		setInherit(si.StdOutput, true)
		setInherit(si.StdErr, true)
	}

	var pi syscall.ProcessInformation
	r, _, e := procCreateProcessW.Call(
		0,
		uintptr(unsafe.Pointer(&cmd[0])),
		0,
		0,
		1,
		uintptr(flags),
		0,
		uintptr(unsafe.Pointer(dirp)),
		uintptr(unsafe.Pointer(&si)),
		uintptr(unsafe.Pointer(&pi)),
	)
	if r == 0 {
		if e == syscall.Errno(0) {
			return fmt.Errorf("CreateProcess failed")
		}
		return e
	}
	syscall.CloseHandle(pi.Thread)
	defer syscall.CloseHandle(pi.Process)
	s, err := syscall.WaitForSingleObject(pi.Process, syscall.INFINITE)
	if err != nil {
		return err
	}
	if s != 0 {
		return fmt.Errorf("wait status %d", s)
	}
	var code uint32
	if err := syscall.GetExitCodeProcess(pi.Process, &code); err != nil {
		return err
	}
	if code != 0 {
		return programExit{Code: code}
	}
	return nil
}

// runInherited starts a program the way the file list starts its viewer.
// cmd.exe /C runs it in the node directory. The caller socket is inheritable
// and blocking, and the new console is a normal window. Stdio is not the socket.
func runInherited(command, dir string, handle uint32) error {
	command = trimSpace(command)
	if command == "" {
		return fmt.Errorf("ExternalEdCmd is not set")
	}
	if handle != 0 {
		prepareHandoff(syscall.Handle(handle))
	}
	comspec := os.Getenv("ComSpec")
	if comspec == "" {
		root := os.Getenv("SystemRoot")
		if root == "" {
			root = `C:\Windows`
		}
		comspec = root + `\System32\cmd.exe`
	}
	full := comspec + " /C " + command
	cmd := make([]uint16, len(full)+32)
	copy(cmd, syscall.StringToUTF16(full))
	var dirp *uint16
	if dir != "" {
		p, err := syscall.UTF16PtrFromString(dir)
		if err != nil {
			return err
		}
		dirp = p
	}
	var si syscall.StartupInfo
	si.Cb = uint32(unsafe.Sizeof(si))
	si.Flags = 0x00000001
	si.ShowWindow = 1
	var pi syscall.ProcessInformation
	inherit := uintptr(0)
	if handle != 0 {
		inherit = 1
	}
	r, _, e := procCreateProcessW.Call(
		0,
		uintptr(unsafe.Pointer(&cmd[0])),
		0,
		0,
		inherit,
		0x00000010,
		0,
		uintptr(unsafe.Pointer(dirp)),
		uintptr(unsafe.Pointer(&si)),
		uintptr(unsafe.Pointer(&pi)),
	)
	if r == 0 {
		if e == syscall.Errno(0) {
			return fmt.Errorf("CreateProcess failed")
		}
		return e
	}
	syscall.CloseHandle(pi.Thread)
	defer syscall.CloseHandle(pi.Process)
	s, err := syscall.WaitForSingleObject(pi.Process, syscall.INFINITE)
	if err != nil {
		return err
	}
	if s != 0 {
		return fmt.Errorf("wait status %d", s)
	}
	var code uint32
	if err := syscall.GetExitCodeProcess(pi.Process, &code); err != nil {
		return err
	}
	if code != 0 {
		return programExit{Code: code}
	}
	return nil
}

func prepareHandoff(h syscall.Handle) {
	setInherit(h, true)
	var nb uint32
	procIoctlsocket.Call(uintptr(h), 0x8004667e, uintptr(unsafe.Pointer(&nb)))
	zero := int32(0)
	procSetSockOpt.Call(uintptr(h), 0xffff, 0x1006, uintptr(unsafe.Pointer(&zero)), 4)
	procSetSockOpt.Call(uintptr(h), 0xffff, 0x1005, uintptr(unsafe.Pointer(&zero)), 4)
}

func dupSocket(h syscall.Handle) (syscall.Handle, error) {
	cur, _, _ := procGetCurrentProcess.Call()
	var dup syscall.Handle
	r, _, e := procDuplicateHandle.Call(
		cur,
		uintptr(h),
		cur,
		uintptr(unsafe.Pointer(&dup)),
		0,
		1,
		duplicateSameAccess,
	)
	if r == 0 {
		if e == syscall.Errno(0) {
			return 0, fmt.Errorf("DuplicateHandle failed")
		}
		return 0, e
	}
	return dup, nil
}

func setInherit(h syscall.Handle, on bool) {
	if h == 0 || h == syscall.InvalidHandle {
		return
	}
	flag := uintptr(0)
	if on {
		flag = handleFlagInherit
	}
	procSetHandleInformation.Call(uintptr(h), handleFlagInherit, flag)
}

func stdHandle(which uint32) syscall.Handle {
	h, _, _ := procGetStdHandle.Call(uintptr(which))
	return syscall.Handle(h)
}

func trimSpace(s string) string {
	i := 0
	j := len(s)
	for i < j && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	for j > i && (s[j-1] == ' ' || s[j-1] == '\t') {
		j--
	}
	return s[i:j]
}
