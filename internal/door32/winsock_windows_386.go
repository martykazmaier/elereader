//go:build windows && 386

package door32

import (
	"errors"
	"io"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// Win32 Winsock. SOCKET is a 32-bit handle on GOARCH=386, which matches
// the value EleBBS writes on line 2 of DOOR32.SYS. The socket is inherited
// and must not be passed to closesocket.

var (
	ws2             = syscall.NewLazyDLL("ws2_32.dll")
	procWSAStartup  = ws2.NewProc("WSAStartup")
	procRecv        = ws2.NewProc("recv")
	procSend        = ws2.NewProc("send")
	procIoctlsocket = ws2.NewProc("ioctlsocket")
	procSetSockOpt  = ws2.NewProc("setsockopt")
)

// WSADATA as laid out by the 32-bit Windows headers.
type wsaData struct {
	Version      uint16
	HighVersion  uint16
	Description  [257]byte
	SystemStatus [129]byte
	MaxSockets   uint16
	MaxUdpDg     uint16
	VendorInfo   *byte
}

var startWinsock sync.Once
var startErr error

func ensureWinsock() error {
	startWinsock.Do(func() {
		var data wsaData
		r, _, _ := procWSAStartup.Call(uintptr(0x0202), uintptr(unsafe.Pointer(&data)))
		if r != 0 {
			startErr = syscall.Errno(r)
		}
	})
	return startErr
}

type sockPort struct {
	s uint32
}

func (p *sockPort) Read(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}
	r, _, e := procRecv.Call(
		uintptr(p.s),
		uintptr(unsafe.Pointer(&b[0])),
		uintptr(len(b)),
		0,
	)
	if uint32(r) == 0xFFFFFFFF {
		if deadErrno(e) {
			return 0, io.EOF
		}
		return 0, ErrTimeout
	}
	if r == 0 {
		return 0, io.EOF
	}
	return int(r), nil
}

func (p *sockPort) Write(b []byte) (int, error) {
	total := 0
	spins := 0
	for total < len(b) {
		r, _, e := procSend.Call(
			uintptr(p.s),
			uintptr(unsafe.Pointer(&b[total])),
			uintptr(len(b)-total),
			0,
		)
		if uint32(r) == 0xFFFFFFFF {
			if deadErrno(e) {
				return total, io.EOF
			}
			spins++
			if spins > 200 {
				return total, ErrTimeout
			}
			time.Sleep(5 * time.Millisecond)
			continue
		}
		spins = 0
		if r == 0 {
			return total, io.ErrShortWrite
		}
		total += int(r)
	}
	return total, nil
}

// Close leaves the SOCKET open. EleBBS still has the caller on this handle.
func (p *sockPort) Close() error { return nil }

// deadErrno reports a socket that will not carry the caller again.
// Would-block and a cleared last-error are not dead: the door stays up
// and reads again. GetLastError is taken from the recv/send call itself.
func deadErrno(e error) bool {
	errno, ok := e.(syscall.Errno)
	if !ok || errno == 0 {
		return false
	}
	switch errno {
	case 10038, 10053, 10054, 10057, 10058:
		return true
	default:
		return false
	}
}

func openTelnet(handle uint32) (Port, func(), error) {
	if handle == 0 {
		return nil, func() {}, errors.New("telnet socket handle is 0")
	}
	if err := ensureWinsock(); err != nil {
		return nil, func() {}, err
	}
	p := &sockPort{s: handle}
	p.Blocking(false)
	return p, func() {}, nil
}

// Blocking switches the caller socket between a normal blocking socket
// and a non-blocking one. A receive timeout is not used: Winsock leaves
// the connection unusable after SO_RCVTIMEO fires. The transfer program
// needs the blocking mode or its first read gives up.
func (p *sockPort) Blocking(on bool) {
	var zero int32
	procSetSockOpt.Call(uintptr(p.s), 0xffff, 0x1006, uintptr(unsafe.Pointer(&zero)), 4)
	procSetSockOpt.Call(uintptr(p.s), 0xffff, 0x1005, uintptr(unsafe.Pointer(&zero)), 4)
	nb := uint32(1)
	if on {
		nb = 0
	}
	procIoctlsocket.Call(uintptr(p.s), 0x8004667e, uintptr(unsafe.Pointer(&nb)))
}
