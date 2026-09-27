//go:build windows

package door32

import (
	"errors"
	"io"
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32                = syscall.NewLazyDLL("kernel32.dll")
	procGetStdHandle        = kernel32.NewProc("GetStdHandle")
	procGetConsoleMode      = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode      = kernel32.NewProc("SetConsoleMode")
	procGetConsoleCP        = kernel32.NewProc("GetConsoleCP")
	procSetConsoleCP        = kernel32.NewProc("SetConsoleCP")
	procGetConsoleOutputCP  = kernel32.NewProc("GetConsoleOutputCP")
	procSetConsoleOutputCP  = kernel32.NewProc("SetConsoleOutputCP")
	procReadFile            = kernel32.NewProc("ReadFile")
	procWriteFile           = kernel32.NewProc("WriteFile")
	procWaitForSingleObject = kernel32.NewProc("WaitForSingleObject")
	procSetCommTimeouts     = kernel32.NewProc("SetCommTimeouts")
)

const (
	enableProcessedInput        = 0x0001
	enableLineInput             = 0x0002
	enableEchoInput             = 0x0004
	enableVirtualTerminalInput  = 0x0200
	enableVirtualTerminalOutput = 0x0004
	disableNewlineAutoReturn    = 0x0008
	stdInputHandle              = 0xFFFFFFF6
	stdOutputHandle             = 0xFFFFFFF5
)

// filePort is a Win32 HANDLE used with ReadFile and WriteFile.
// Close does not close the handle. EleBBS still owns a serial port.
type filePort struct {
	h    syscall.Handle
	poll bool
}

func (p *filePort) Read(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}
	if p.poll {
		r, _, _ := procWaitForSingleObject.Call(uintptr(p.h), 200)
		if r == 0x102 {
			return 0, ErrTimeout
		}
	}
	var done uint32
	r, _, e := procReadFile.Call(
		uintptr(p.h),
		uintptr(unsafe.Pointer(&b[0])),
		uintptr(len(b)),
		uintptr(unsafe.Pointer(&done)),
		0,
	)
	if r == 0 {
		if e == syscall.Errno(0) {
			return 0, io.EOF
		}
		return 0, e
	}
	if done == 0 {
		return 0, ErrTimeout
	}
	return int(done), nil
}

func (p *filePort) Write(b []byte) (int, error) {
	total := 0
	for total < len(b) {
		var done uint32
		r, _, e := procWriteFile.Call(
			uintptr(p.h),
			uintptr(unsafe.Pointer(&b[total])),
			uintptr(len(b)-total),
			uintptr(unsafe.Pointer(&done)),
			0,
		)
		if r == 0 {
			if e == syscall.Errno(0) {
				return total, io.ErrShortWrite
			}
			return total, e
		}
		if done == 0 {
			return total, io.ErrShortWrite
		}
		total += int(done)
	}
	return total, nil
}

func (p *filePort) Close() error { return nil }

// Blocking selects a full wait for a transfer program, or the short
// timeout the reader uses so it can be stopped between reads.
func (p *filePort) Blocking(on bool) {
	if p.poll {
		return
	}
	var to [5]uint32
	if !on {
		to[0] = 0xFFFFFFFF
		to[2] = 200
	}
	procSetCommTimeouts.Call(uintptr(p.h), uintptr(unsafe.Pointer(&to[0])))
}

func openSerial(handle uint32) (Port, func(), error) {
	if handle == 0 {
		return nil, func() {}, errors.New("serial comm handle is 0")
	}
	h := syscall.Handle(handle)
	var to [5]uint32
	to[0] = 0xFFFFFFFF
	to[2] = 200
	procSetCommTimeouts.Call(uintptr(h), uintptr(unsafe.Pointer(&to[0])))
	return &filePort{h: h}, func() {}, nil
}

type stdioPort struct {
	in  io.Reader
	out io.Writer
}

func (p *stdioPort) Read(b []byte) (int, error)  { return p.in.Read(b) }
func (p *stdioPort) Write(b []byte) (int, error) { return p.out.Write(b) }
func (p *stdioPort) Close() error                { return nil }

func openLocal() (Port, func(), error) {
	in := stdHandle(stdInputHandle)
	out := stdHandle(stdOutputHandle)
	var inMode, outMode uint32
	r, _, _ := procGetConsoleMode.Call(uintptr(in), uintptr(unsafe.Pointer(&inMode)))
	if r == 0 {
		return &stdioPort{os.Stdin, os.Stdout}, func() {}, nil
	}
	_, _, _ = procGetConsoleMode.Call(uintptr(out), uintptr(unsafe.Pointer(&outMode)))

	rawIn := (inMode &^ (enableProcessedInput | enableLineInput | enableEchoInput)) | enableVirtualTerminalInput
	rawOut := outMode | enableVirtualTerminalOutput | disableNewlineAutoReturn
	procSetConsoleMode.Call(uintptr(in), uintptr(rawIn))
	procSetConsoleMode.Call(uintptr(out), uintptr(rawOut))

	inCP, _, _ := procGetConsoleCP.Call()
	outCP, _, _ := procGetConsoleOutputCP.Call()
	procSetConsoleCP.Call(437)
	procSetConsoleOutputCP.Call(437)

	port := &filePort{h: in, poll: true}
	// Reads come from stdin. Writes must go to stdout, which is a different handle.
	split := &splitPort{r: port, w: &filePort{h: out}}
	cleanup := func() {
		procSetConsoleMode.Call(uintptr(in), uintptr(inMode))
		procSetConsoleMode.Call(uintptr(out), uintptr(outMode))
		procSetConsoleCP.Call(inCP)
		procSetConsoleOutputCP.Call(outCP)
	}
	return split, cleanup, nil
}

type splitPort struct {
	r io.Reader
	w io.Writer
}

func (p *splitPort) Read(b []byte) (int, error)  { return p.r.Read(b) }
func (p *splitPort) Write(b []byte) (int, error) { return p.w.Write(b) }
func (p *splitPort) Close() error                { return nil }

func stdHandle(which uint32) syscall.Handle {
	h, _, _ := procGetStdHandle.Call(uintptr(which))
	return syscall.Handle(h)
}
