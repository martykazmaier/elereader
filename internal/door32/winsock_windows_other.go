//go:build windows && !386

package door32

import "errors"

func openTelnet(handle uint32) (Port, func(), error) {
	return nil, func() {}, errors.New("DOOR32 telnet needs the 32-bit build (GOARCH=386) so the Win32 SOCKET handle can be used")
}
