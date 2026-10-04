//go:build linux

package door32

import (
	"errors"
	"io"
	"os"
	"syscall"
	"time"
	"unsafe"
)

// fdPort is a descriptor the BBS passed to the door: a telnet socket or a
// serial tty. Reads time out so the reader can be paused for a child
// program. Close leaves the descriptor open because the BBS owns it.
type fdPort struct {
	fd int
	f  *os.File
}

func newFDPort(fd int) (*fdPort, error) {
	if err := syscall.SetNonblock(fd, true); err != nil {
		return nil, err
	}
	return &fdPort{fd: fd, f: os.NewFile(uintptr(fd), "door32")}, nil
}

func (p *fdPort) Read(b []byte) (int, error) {
	if err := p.f.SetReadDeadline(time.Now().Add(200 * time.Millisecond)); err != nil {
		n, err := p.f.Read(b)
		if n == 0 && err == nil {
			return 0, ErrTimeout
		}
		return n, err
	}
	n, err := p.f.Read(b)
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return n, ErrTimeout
	}
	return n, err
}

func (p *fdPort) Write(b []byte) (int, error) { return p.f.Write(b) }
func (p *fdPort) Close() error                { return nil }

// Blocking hands a child program an ordinary blocking descriptor, and puts
// the door's non-blocking mode back afterward.
func (p *fdPort) Blocking(on bool) {
	_ = syscall.SetNonblock(p.fd, !on)
}

func openTelnet(handle uint32) (Port, func(), error) {
	if handle == 0 {
		return nil, func() {}, errors.New("telnet socket handle is 0")
	}
	p, err := newFDPort(int(handle))
	if err != nil {
		return nil, func() {}, err
	}
	return p, func() { _ = syscall.SetNonblock(int(handle), false) }, nil
}

func openSerial(handle uint32) (Port, func(), error) {
	if handle == 0 {
		return nil, func() {}, errors.New("serial comm handle is 0")
	}
	p, err := newFDPort(int(handle))
	if err != nil {
		return nil, func() {}, err
	}
	return p, func() { _ = syscall.SetNonblock(int(handle), false) }, nil
}

type stdioPort struct {
	in  io.Reader
	out io.Writer
}

func (p *stdioPort) Read(b []byte) (int, error)  { return p.in.Read(b) }
func (p *stdioPort) Write(b []byte) (int, error) { return p.out.Write(b) }
func (p *stdioPort) Close() error                { return nil }

func openLocal() (Port, func(), error) {
	var old syscall.Termios
	if ioctl(0, syscall.TCGETS, &old) != nil {
		return &stdioPort{os.Stdin, os.Stdout}, func() {}, nil
	}
	raw := old
	raw.Iflag &^= syscall.ICRNL | syscall.IXON | syscall.ISTRIP | syscall.INLCR | syscall.IGNCR
	raw.Lflag &^= syscall.ECHO | syscall.ICANON | syscall.ISIG | syscall.IEXTEN
	raw.Cc[syscall.VMIN] = 0
	raw.Cc[syscall.VTIME] = 2
	if err := ioctl(0, syscall.TCSETS, &raw); err != nil {
		return &stdioPort{os.Stdin, os.Stdout}, func() {}, nil
	}
	in := &ttyReader{}
	cleanup := func() { _ = ioctl(0, syscall.TCSETS, &old) }
	return &stdioPort{in, os.Stdout}, cleanup, nil
}

// ttyReader reads the raw terminal. VTIME makes an idle read return
// after 200ms with nothing, which is reported as a timeout.
type ttyReader struct{}

func (ttyReader) Read(b []byte) (int, error) {
	n, err := syscall.Read(0, b)
	if n <= 0 && (err == nil || err == syscall.EAGAIN || err == syscall.EINTR) {
		return 0, ErrTimeout
	}
	if err != nil {
		return 0, err
	}
	return n, nil
}

func ioctl(fd int, req uintptr, t *syscall.Termios) error {
	_, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), req, uintptr(unsafe.Pointer(t)))
	if e != 0 {
		return e
	}
	return nil
}
