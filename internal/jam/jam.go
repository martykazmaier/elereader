// Package jam reads EleBBS JAM message bases.
// JAM(mbp) - Copyright 1993 Joaquim Homrighausen, Andrew Milner,
// Mats Birch, Mats Wallin. ALL RIGHTS RESERVED.
package jam

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode"
)

const (
	sigJAM   = 0x004D414A // "JAM\0" little-endian
	hdrInfo  = 1024
	fixedLen = 76 // signature through Cost

	attrLocal    = 0x00000001
	attrPrivate  = 0x00000004
	attrRead     = 0x00000008
	attrTypeLoc  = 0x00800000
	attrTypeEcho = 0x01000000
	attrTypeNet  = 0x02000000
	attrNoDisp   = 0x20000000
	attrDeleted  = 0x80000000

	subOAddr    = 0
	subDAddr    = 1
	subSender   = 2
	subReceiver = 3
	subMsgID    = 4
	subReplyID  = 5
	subSubject  = 6
	subPID      = 7
	subFile     = 9
	subKludge   = 2000
	subSeenBy   = 2001
	subPath     = 2002
	subTZUTC    = 2004

	attrFile = 0x00002000
)

// Area kinds match EleBBS message-area types.
const (
	AreaLocal AreaKind = iota
	AreaEcho
	AreaNetmail
	AreaEmail
)

// AreaKind is how EleBBS classifies the conference being read.
type AreaKind int

func (k AreaKind) Label() string {
	switch k {
	case AreaEcho:
		return "Echomail"
	case AreaNetmail:
		return "Netmail"
	case AreaEmail:
		return "Email"
	default:
		return "Local"
	}
}

// ParseKind reads a type name from a command line or config file.
func ParseKind(s string) (AreaKind, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "local":
		return AreaLocal, true
	case "echo", "echomail":
		return AreaEcho, true
	case "net", "netmail":
		return AreaNetmail, true
	case "email", "e-mail", "internet":
		return AreaEmail, true
	default:
		return AreaLocal, false
	}
}

// Header is one visible message. Text is loaded separately.
type Header struct {
	Number  uint32
	From    string
	To      string
	Subject string
	Origin  string
	Dest    string
	MsgID   string
	Kludges []string
	When    time.Time
	Attr    uint32
	Offset  uint32
	TxtLen  uint32
	HdrAt   uint32
	Files   []string
}

func (h Header) Private() bool { return h.Attr&attrPrivate != 0 }
func (h Header) Read() bool    { return h.Attr&attrRead != 0 }
func (h Header) Deleted() bool { return h.Attr&attrDeleted != 0 }

// Flags is a short attribute tag for the message view.
func (h Header) Flags() string {
	var b strings.Builder
	if h.Private() {
		b.WriteString("Private ")
	}
	switch {
	case h.Attr&attrTypeEcho != 0:
		b.WriteString("Echo")
	case h.Attr&attrTypeNet != 0:
		b.WriteString("Netmail")
	case h.Attr&attrTypeLoc != 0:
		b.WriteString("Local")
	}
	return strings.TrimSpace(b.String())
}

// Base is an open JAM conference. Files are opened shared so EleBBS can
// keep the area open while a reply is written.
type Base struct {
	path     string
	jhr      *os.File
	jdt      *os.File
	jdx      *os.File
	jlr      *os.File
	baseMsg  uint32
	writable bool
}

// Open opens path without an extension (.JHR .JDT .JDX .JLR).
// Read/write is used when the base allows it, so a reply can be saved
// while EleBBS keeps the area open.
func Open(path string) (*Base, error) {
	jhr, jhrW, err := openShared(path, ".JHR", ".jhr")
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path+".JHR", err)
	}
	b := &Base{path: path, jhr: jhr, baseMsg: 1}
	var jdtW, jdxW bool
	b.jdt, jdtW, _ = openShared(path, ".JDT", ".jdt")
	var idxErr error
	b.jdx, jdxW, idxErr = openShared(path, ".JDX", ".jdx")
	if idxErr != nil {
		jhr.Close()
		if b.jdt != nil {
			b.jdt.Close()
		}
		return nil, fmt.Errorf("open %s index: %w", path, idxErr)
	}
	b.jlr, _, _ = openShared(path, ".JLR", ".jlr")
	b.writable = jhrW && jdxW && (b.jdt == nil || jdtW)
	var info [24]byte
	if _, err := io.ReadFull(jhr, info[:]); err != nil {
		b.Close()
		return nil, fmt.Errorf("%s header: %w", path, err)
	}
	if binary.LittleEndian.Uint32(info[0:4]) != sigJAM {
		b.Close()
		return nil, fmt.Errorf("%s is not a JAM base", path)
	}
	if n := binary.LittleEndian.Uint32(info[20:24]); n != 0 {
		b.baseMsg = n
	}
	return b, nil
}

func openShared(path, a, c string) (*os.File, bool, error) {
	var missing error
	for _, ext := range []string{a, c} {
		name := path + ext
		f, err := os.OpenFile(name, os.O_RDWR, 0644)
		if err == nil {
			return f, true, nil
		}
		if os.IsNotExist(err) {
			missing = err
			continue
		}
		f, err2 := os.Open(name)
		if err2 == nil {
			return f, false, nil
		}
		return nil, false, err
	}
	if missing == nil {
		missing = os.ErrNotExist
	}
	return nil, false, missing
}

// Close releases the base. It does not modify it.
func (b *Base) Close() {
	for _, f := range []*os.File{b.jhr, b.jdt, b.jdx, b.jlr} {
		if f != nil {
			f.Close()
		}
	}
}

// HighRead is the highest message number this user has read.
// Zero means the user has no lastread record.
func (b *Base) HighRead(names ...string) uint32 {
	if b.jlr == nil {
		return 0
	}
	st, err := b.jlr.Stat()
	if err != nil || st.Size() < 16 {
		return 0
	}
	want := map[uint32]bool{}
	for _, n := range names {
		if n != "" {
			want[CRC32String(n)] = true
		}
	}
	buf := make([]byte, st.Size())
	_, _ = b.jlr.ReadAt(buf, 0)
	var best uint32
	for off := 0; off+16 <= len(buf); off += 16 {
		crc := binary.LittleEndian.Uint32(buf[off : off+4])
		if !want[crc] {
			continue
		}
		high := binary.LittleEndian.Uint32(buf[off+12 : off+16])
		if high > best {
			best = high
		}
	}
	return best
}

// List returns messages that can be shown, oldest first.
func (b *Base) List() ([]Header, error) {
	st, err := b.jdx.Stat()
	if err != nil {
		return nil, err
	}
	n := int(st.Size() / 8)
	out := make([]Header, 0, n)
	var rec [8]byte
	for i := 0; i < n; i++ {
		if _, err := b.jdx.ReadAt(rec[:], int64(i)*8); err != nil {
			return out, err
		}
		userCRC := binary.LittleEndian.Uint32(rec[0:4])
		off := binary.LittleEndian.Uint32(rec[4:8])
		if userCRC == 0xFFFFFFFF && off == 0xFFFFFFFF {
			continue
		}
		h, ok, err := b.readHeader(off, b.baseMsg+uint32(i))
		if err != nil {
			return out, err
		}
		if !ok || h.Deleted() || h.Attr&attrNoDisp != 0 {
			continue
		}
		out = append(out, h)
	}
	return out, nil
}

func (b *Base) readHeader(off, number uint32) (Header, bool, error) {
	if off < hdrInfo {
		return Header{}, false, nil
	}
	var raw [fixedLen]byte
	if _, err := b.jhr.ReadAt(raw[:], int64(off)); err != nil {
		if err == io.EOF {
			return Header{}, false, nil
		}
		return Header{}, false, err
	}
	if binary.LittleEndian.Uint32(raw[0:4]) != sigJAM {
		return Header{}, false, nil
	}
	subLen := binary.LittleEndian.Uint32(raw[8:12])
	h := Header{
		Number: number,
		HdrAt:  off,
		When:   time.Unix(int64(binary.LittleEndian.Uint32(raw[36:40])), 0).UTC(),
		Attr:   binary.LittleEndian.Uint32(raw[52:56]),
		Offset: binary.LittleEndian.Uint32(raw[60:64]),
		TxtLen: binary.LittleEndian.Uint32(raw[64:68]),
	}
	if subLen > 0 && subLen < 1<<20 {
		buf := make([]byte, subLen)
		if _, err := b.jhr.ReadAt(buf, int64(off)+fixedLen); err == nil {
			parseSubs(buf, &h)
		}
	}
	return h, true, nil
}

func parseSubs(buf []byte, h *Header) {
	for len(buf) >= 8 {
		id := binary.LittleEndian.Uint16(buf[0:2])
		n := binary.LittleEndian.Uint32(buf[4:8])
		buf = buf[8:]
		if uint32(len(buf)) < n {
			return
		}
		val := cleanField(buf[:n])
		buf = buf[n:]
		switch id {
		case subOAddr:
			h.Origin = val
		case subDAddr:
			h.Dest = val
		case subMsgID:
			h.MsgID = val
			h.Kludges = append(h.Kludges, "MSGID: "+val)
		case subReplyID:
			h.Kludges = append(h.Kludges, "REPLY: "+val)
		case subPID:
			h.Kludges = append(h.Kludges, "PID: "+val)
		case subKludge:
			h.Kludges = append(h.Kludges, val)
		case subSeenBy:
			h.Kludges = append(h.Kludges, "SEEN-BY: "+val)
		case subPath:
			h.Kludges = append(h.Kludges, "PATH: "+val)
		case subTZUTC:
			h.Kludges = append(h.Kludges, "TZUTC: "+val)
		case subSender:
			h.From = val
		case subReceiver:
			h.To = val
		case subSubject:
			h.Subject = val
		case subFile:
			if val != "" {
				h.Files = append(h.Files, val)
			}
		}
	}
}

func cleanField(b []byte) string {
	if i := indexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return strings.TrimRightFunc(string(b), unicode.IsSpace)
}

func indexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}

// MessageID is the MSGID from the header, or from a ^AMSGID kludge in the
// text for bases that keep kludges there.
func (b *Base) MessageID(h Header) string {
	if h.MsgID != "" {
		return h.MsgID
	}
	raw, err := b.Text(h)
	if err != nil {
		return ""
	}
	for _, ln := range strings.FieldsFunc(string(raw), func(r rune) bool { return r == '\r' || r == '\n' }) {
		if strings.HasPrefix(ln, "\x01MSGID:") {
			return strings.TrimSpace(ln[len("\x01MSGID:"):])
		}
	}
	return ""
}

// Text loads the message body. Missing text yields an empty slice.
func (b *Base) Text(h Header) ([]byte, error) {
	if b.jdt == nil || h.TxtLen == 0 {
		return nil, nil
	}
	if h.TxtLen > 512*1024 {
		return nil, fmt.Errorf("message %d text is too large", h.Number)
	}
	buf := make([]byte, h.TxtLen)
	_, err := b.jdt.ReadAt(buf, int64(h.Offset))
	if err != nil && err != io.EOF {
		return nil, err
	}
	return buf, nil
}
