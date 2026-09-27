package door32

import (
	"errors"
	"fmt"
	"io"
)

// Port is the caller channel. Close must not drop a BBS-owned socket.
type Port interface {
	io.ReadWriteCloser
}

// ErrTimeout means a read woke up with no caller input.
var ErrTimeout = errors.New("read timed out")

// Open connects to the caller described by the drop file.
// Telnet uses ws2_32 send/recv on the inherited 32-bit SOCKET.
// Serial uses ReadFile/WriteFile. Local uses the console.
// The returned cleanup restores the local console. It does not close
// a socket or comm handle that belongs to EleBBS.
func Open(d Drop) (Port, func(), error) {
	switch d.CommType {
	case CommLocal:
		return openLocal()
	case CommSerial:
		return openSerial(d.Handle)
	case CommTelnet:
		return openTelnet(d.Handle)
	default:
		return nil, func() {}, fmt.Errorf("unknown comm type %d", d.CommType)
	}
}
