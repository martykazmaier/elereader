package ui

import (
	"fmt"
	"io"
)

const (
	attrReset = "\x1b[0m"
	attrFrame = "\x1b[1;36m"
	attrHead  = "\x1b[1;33m"
	attrNorm  = "\x1b[0;37m"
	attrNew   = "\x1b[1;37m"
	attrBar   = "\x1b[1;37;44m"
)

const (
	chTL = 0xC9
	chTR = 0xBB
	chBL = 0xC8
	chBR = 0xBC
	chJL = 0xCC
	chJR = 0xB9
	chH  = 0xCD
	chV  = 0xBA
)

// Screen writes an 80x24 ANSI frame. Wrap is turned off so a full row
// does not scroll the caller.
type Screen struct {
	w io.Writer
}

func (s *Screen) begin() {
	s.text("\x1b[0m\x1b[?7l\x1b[?25l\x1b[2J")
}

func (s *Screen) text(v string) {
	_, _ = s.w.Write([]byte(v))
}

func (s *Screen) bytes(p []byte) {
	if len(p) > 0 {
		_, _ = s.w.Write(p)
	}
}

func (s *Screen) cup(y, x int) {
	s.text(fmt.Sprintf("\x1b[%d;%dH", y, x))
}

func (s *Screen) rule(y int, left, right byte, title string) {
	line := make([]byte, 80)
	line[0] = left
	for i := 1; i < 79; i++ {
		line[i] = chH
	}
	line[79] = right
	if title != "" {
		tb := []byte(" " + title + " ")
		if len(tb) > 74 {
			tb = tb[:74]
		}
		copy(line[2:], tb)
	}
	s.cup(y, 1)
	s.text(attrFrame)
	s.bytes(line)
	s.text(attrReset)
	s.hold()
}

// row paints one full 80-column line. Message ANSI is already resolved
// into the text, so this does not run the message's own cursor codes.
func (s *Screen) row(y int, text []byte) {
	text = padVisible(string(text), ansiWidth)
	s.cup(y, 1)
	s.text(attrNorm)
	s.bytes(text)
	s.text(attrReset)
	s.hold()
}

func (s *Screen) content(y int, color string, text []byte) {
	text = padVisible(string(text), contentWidth)
	s.cup(y, 1)
	s.text(attrFrame)
	s.bytes([]byte{chV})
	s.text(color)
	s.bytes(text)
	s.text(attrFrame)
	s.bytes([]byte{chV})
	s.text(attrReset)
	s.hold()
}

func (s *Screen) hold() {
	s.text("\x1b[?25l\x1b[1;1H")
}
