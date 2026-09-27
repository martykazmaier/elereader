// Package door32 reads an EleBBS DOOR32.SYS drop file and opens the
// inherited Win32 comm handle. Telnet sessions use the 32-bit Winsock
// SOCKET from line 2; this process never closes that socket.
package door32

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

// Comm types from DOOR32 revision 1.
const (
	CommLocal  = 0
	CommSerial = 1
	CommTelnet = 2
)

// Emulation values from DOOR32 revision 1.
const (
	EmuASCII  = 0
	EmuANSI   = 1
	EmuAvatar = 2
	EmuRIP    = 3
	EmuMaxGfx = 4
)

// Drop is one DOOR32.SYS.
type Drop struct {
	CommType   int
	Handle     uint32
	Baud       int
	BBSID      string
	UserNumber int
	RealName   string
	Alias      string
	Security   int
	TimeLeft   int
	Emulation  int
	Node       int
	Path       string
	Started    time.Time
}

// ANSI reports whether the caller can take ANSI. Avatar, RIP, and Max
// Graphics fall back to ANSI, per the DOOR32 spec.
func (d Drop) ANSI() bool {
	return d.Emulation != EmuASCII
}

// Who is the name shown on the status line.
func (d Drop) Who() string {
	if d.Alias != "" {
		return d.Alias
	}
	if d.RealName != "" {
		return d.RealName
	}
	return "Guest"
}

// Remaining is minutes still available. Zero or negative time-left in the
// drop file means the BBS did not impose a limit.
func (d Drop) Remaining(now time.Time) (mins int, unlimited bool) {
	if d.TimeLeft <= 0 {
		return 0, true
	}
	used := int(now.Sub(d.Started).Minutes())
	left := d.TimeLeft - used
	if left < 0 {
		left = 0
	}
	return left, false
}

// Parse reads a DOOR32.SYS file. The handle is the Win32 SOCKET or COMM
// handle and is kept as uint32.
func Parse(path string) (Drop, error) {
	f, err := os.Open(path)
	if err != nil {
		return Drop{}, err
	}
	defer f.Close()
	return ParseReader(path, f)
}

// ParseReader parses the 11-line DOOR32 revision 1 layout.
func ParseReader(path string, r io.Reader) (Drop, error) {
	var lines []string
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		lines = append(lines, strings.TrimSpace(sc.Text()))
	}
	if err := sc.Err(); err != nil {
		return Drop{}, err
	}
	if len(lines) < 11 {
		return Drop{}, fmt.Errorf("%s: need 11 lines, got %d", path, len(lines))
	}
	num := func(i int, name string) (int, error) {
		n, err := strconv.Atoi(lines[i])
		if err != nil {
			return 0, fmt.Errorf("%s: line %d (%s): %w", path, i+1, name, err)
		}
		return n, nil
	}
	comm, err := num(0, "comm type")
	if err != nil {
		return Drop{}, err
	}
	if comm < CommLocal || comm > CommTelnet {
		return Drop{}, fmt.Errorf("%s: comm type %d is not 0, 1, or 2", path, comm)
	}
	handle64, err := strconv.ParseUint(lines[1], 10, 32)
	if err != nil {
		return Drop{}, fmt.Errorf("%s: line 2 (socket handle): %w", path, err)
	}
	baud, err := num(2, "baud")
	if err != nil {
		return Drop{}, err
	}
	userNum, err := num(4, "user record")
	if err != nil {
		return Drop{}, err
	}
	level, err := num(7, "security")
	if err != nil {
		return Drop{}, err
	}
	left, err := num(8, "time left")
	if err != nil {
		return Drop{}, err
	}
	emu, err := num(9, "emulation")
	if err != nil {
		return Drop{}, err
	}
	node, err := num(10, "node")
	if err != nil {
		return Drop{}, err
	}
	return Drop{
		CommType:   comm,
		Handle:     uint32(handle64),
		Baud:       baud,
		BBSID:      lines[3],
		UserNumber: userNum,
		RealName:   lines[5],
		Alias:      lines[6],
		Security:   level,
		TimeLeft:   left,
		Emulation:  emu,
		Node:       node,
		Path:       path,
		Started:    time.Now(),
	}, nil
}
