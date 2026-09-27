package jam

import (
	"encoding/binary"
	"fmt"
	"os"
	"time"
)

// Outgoing is a message written back into the JAM base.
type Outgoing struct {
	From     string
	To       string
	Subject  string
	Text     []byte
	Kind     AreaKind
	Private  bool
	ReplyTo  uint32
	FromAddr string
	ToAddr   string
	Files    []string
	Attach   bool
}

// Post appends a message. The returned number is the new JAM message number.
func (b *Base) Post(msg Outgoing) (uint32, error) {
	if !b.writable {
		return 0, fmt.Errorf("message base is read-only")
	}
	if b.jdt == nil {
		f, err := os.OpenFile(b.path+".JDT", os.O_RDWR|os.O_CREATE, 0644)
		if err != nil {
			return 0, err
		}
		b.jdt = f
	}
	st, err := b.jdx.Stat()
	if err != nil {
		return 0, err
	}
	count := uint32(st.Size() / 8)
	number := b.baseMsg + count

	txtSt, err := b.jdt.Stat()
	if err != nil {
		return 0, err
	}
	textAt := uint32(txtSt.Size())
	if len(msg.Text) > 0 {
		if _, err := b.jdt.WriteAt(msg.Text, int64(textAt)); err != nil {
			return 0, err
		}
	}

	subs := field(subSender, clip(msg.From, 100))
	subs = append(subs, field(subReceiver, clip(msg.To, 100))...)
	subs = append(subs, field(subSubject, clip(msg.Subject, 100))...)
	subs = append(subs, field(subPID, "Elereader")...)
	if msg.FromAddr != "" {
		subs = append(subs, field(subOAddr, clip(msg.FromAddr, 100))...)
	}
	if msg.ToAddr != "" {
		subs = append(subs, field(subDAddr, clip(msg.ToAddr, 100))...)
	}
	for _, name := range msg.Files {
		if name != "" {
			subs = append(subs, field(subFile, clip(name, 100))...)
		}
	}

	hdrSt, err := b.jhr.Stat()
	if err != nil {
		return 0, err
	}
	hdrAt := hdrSt.Size()
	if hdrAt < hdrInfo {
		hdrAt = hdrInfo
	}
	raw := make([]byte, fixedLen+len(subs))
	attr := attrs(msg.Kind, msg.Private, len(msg.Files) > 0 || msg.Attach)
	putFixed(raw, uint32(len(subs)), uint32(time.Now().Unix()), number, attr, textAt, uint32(len(msg.Text)), msg.ReplyTo)
	copy(raw[fixedLen:], subs)
	if _, err := b.jhr.WriteAt(raw, hdrAt); err != nil {
		return 0, err
	}

	idx := make([]byte, 8)
	binary.LittleEndian.PutUint32(idx[0:], CRC32String(msg.To))
	binary.LittleEndian.PutUint32(idx[4:], uint32(hdrAt))
	if _, err := b.jdx.WriteAt(idx, int64(count)*8); err != nil {
		return 0, err
	}
	if msg.ReplyTo != 0 {
		_ = b.linkReply(msg.ReplyTo, number)
	}
	if err := b.touchInfo(1); err != nil {
		return number, err
	}
	return number, nil
}

// Seen records that this user has read the message. Messages addressed to
// the user get the received bit. The lastread high water moves forward.
func (b *Base) Seen(h Header, userID uint32, toUser bool, names ...string) (uint32, uint32, error) {
	attr := h.Attr
	if toUser && attr&attrRead == 0 {
		if !b.writable || h.HdrAt < hdrInfo {
			return 0, attr, fmt.Errorf("message base is read-only")
		}
		var err error
		attr, err = b.markReceived(h.HdrAt)
		if err != nil {
			return 0, h.Attr, err
		}
	}
	high, err := b.advanceLastRead(h.Number, userID, names...)
	if err != nil {
		return 0, attr, err
	}
	return high, attr, nil
}

// Delete marks a message deleted and drops it from the active count.
func (b *Base) Delete(h Header) error {
	if !b.writable || h.HdrAt < hdrInfo {
		return fmt.Errorf("message base is read-only")
	}
	var buf [4]byte
	if _, err := b.jhr.ReadAt(buf[:], int64(h.HdrAt)+52); err != nil {
		return err
	}
	attr := binary.LittleEndian.Uint32(buf[:])
	if attr&attrDeleted != 0 {
		return nil
	}
	binary.LittleEndian.PutUint32(buf[:], attr|attrDeleted)
	if _, err := b.jhr.WriteAt(buf[:], int64(h.HdrAt)+52); err != nil {
		return err
	}
	return b.touchInfo(-1)
}

func (b *Base) markReceived(at uint32) (uint32, error) {
	var buf [4]byte
	if _, err := b.jhr.ReadAt(buf[:], int64(at)+52); err != nil {
		return 0, err
	}
	attr := binary.LittleEndian.Uint32(buf[:]) | attrRead
	binary.LittleEndian.PutUint32(buf[:], attr)
	if _, err := b.jhr.WriteAt(buf[:], int64(at)+52); err != nil {
		return 0, err
	}
	if _, err := b.jhr.ReadAt(buf[:], int64(at)+12); err == nil {
		n := binary.LittleEndian.Uint32(buf[:])
		if n == 0xFFFFFFFF {
			n = 0
		}
		binary.LittleEndian.PutUint32(buf[:], n+1)
		_, _ = b.jhr.WriteAt(buf[:], int64(at)+12)
	}
	if _, err := b.jhr.ReadAt(buf[:], int64(at)+44); err == nil && binary.LittleEndian.Uint32(buf[:]) == 0 {
		binary.LittleEndian.PutUint32(buf[:], uint32(time.Now().Unix()))
		_, _ = b.jhr.WriteAt(buf[:], int64(at)+44)
	}
	return attr, nil
}

func (b *Base) advanceLastRead(number, userID uint32, names ...string) (uint32, error) {
	var order []uint32
	want := map[uint32]bool{}
	for _, n := range names {
		if n == "" {
			continue
		}
		c := CRC32String(n)
		if want[c] {
			continue
		}
		want[c] = true
		order = append(order, c)
	}
	if len(order) == 0 || number == 0 {
		return b.HighRead(names...), nil
	}
	f, err := b.lastreadFile()
	if err != nil {
		return 0, err
	}
	st, err := f.Stat()
	if err != nil {
		return 0, err
	}
	buf := make([]byte, st.Size())
	if len(buf) > 0 {
		_, _ = f.ReadAt(buf, 0)
	}
	seen := map[uint32]bool{}
	var best uint32
	for off := 0; off+16 <= len(buf); off += 16 {
		crc := binary.LittleEndian.Uint32(buf[off : off+4])
		if !want[crc] {
			continue
		}
		id := binary.LittleEndian.Uint32(buf[off+4 : off+8])
		high := binary.LittleEndian.Uint32(buf[off+12 : off+16])
		if userID != 0 && id != 0 && id != userID {
			if high > best {
				best = high
			}
			continue
		}
		seen[crc] = true
		if userID != 0 && id == 0 {
			binary.LittleEndian.PutUint32(buf[off+4:off+8], userID)
		}
		last := binary.LittleEndian.Uint32(buf[off+8 : off+12])
		if number > last {
			binary.LittleEndian.PutUint32(buf[off+8:off+12], number)
		}
		if number > high {
			high = number
			binary.LittleEndian.PutUint32(buf[off+12:off+16], high)
		}
		if high > best {
			best = high
		}
		if _, err := f.WriteAt(buf[off:off+16], int64(off)); err != nil {
			return 0, err
		}
	}
	end := st.Size()
	for _, crc := range order {
		if seen[crc] {
			continue
		}
		rec := make([]byte, 16)
		binary.LittleEndian.PutUint32(rec[0:], crc)
		binary.LittleEndian.PutUint32(rec[4:], userID)
		binary.LittleEndian.PutUint32(rec[8:], number)
		binary.LittleEndian.PutUint32(rec[12:], number)
		if _, err := f.WriteAt(rec, end); err != nil {
			return 0, err
		}
		end += 16
		if number > best {
			best = number
		}
	}
	return best, nil
}

func (b *Base) lastreadFile() (*os.File, error) {
	if b.jlr != nil {
		return b.jlr, nil
	}
	f, err := os.OpenFile(b.path+".JLR", os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}
	b.jlr = f
	return f, nil
}

func attrs(kind AreaKind, private, file bool) uint32 {
	attr := uint32(attrLocal)
	switch kind {
	case AreaEcho:
		attr |= attrTypeEcho
	case AreaNetmail:
		attr |= attrTypeNet | attrPrivate
	case AreaEmail:
		attr |= attrTypeLoc | attrPrivate
	default:
		attr |= attrTypeLoc
		if private {
			attr |= attrPrivate
		}
	}
	if file {
		attr |= attrFile
	}
	return attr
}

func putFixed(h []byte, subLen, date, number, attr, textAt, textLen, replyTo uint32) {
	binary.LittleEndian.PutUint32(h[0:], sigJAM)
	binary.LittleEndian.PutUint16(h[4:], 1)
	binary.LittleEndian.PutUint32(h[8:], subLen)
	binary.LittleEndian.PutUint32(h[16:], 0xFFFFFFFF)
	binary.LittleEndian.PutUint32(h[20:], 0xFFFFFFFF)
	binary.LittleEndian.PutUint32(h[24:], replyTo)
	binary.LittleEndian.PutUint32(h[36:], date)
	binary.LittleEndian.PutUint32(h[48:], number)
	binary.LittleEndian.PutUint32(h[52:], attr)
	binary.LittleEndian.PutUint32(h[60:], textAt)
	binary.LittleEndian.PutUint32(h[64:], textLen)
	binary.LittleEndian.PutUint32(h[68:], 0xFFFFFFFF)
}

func (b *Base) linkReply(orig, next uint32) error {
	off, ok := b.headerOffset(orig)
	if !ok {
		return nil
	}
	var buf [4]byte
	if _, err := b.jhr.ReadAt(buf[:], int64(off)+28); err != nil {
		return err
	}
	if binary.LittleEndian.Uint32(buf[:]) == 0 {
		binary.LittleEndian.PutUint32(buf[:], next)
		_, err := b.jhr.WriteAt(buf[:], int64(off)+28)
		return err
	}
	cur := binary.LittleEndian.Uint32(buf[:])
	for i := 0; i < 128; i++ {
		coff, ok := b.headerOffset(cur)
		if !ok {
			return nil
		}
		if _, err := b.jhr.ReadAt(buf[:], int64(coff)+32); err != nil {
			return err
		}
		n := binary.LittleEndian.Uint32(buf[:])
		if n == 0 {
			binary.LittleEndian.PutUint32(buf[:], next)
			_, err := b.jhr.WriteAt(buf[:], int64(coff)+32)
			return err
		}
		cur = n
	}
	return nil
}

func (b *Base) headerOffset(number uint32) (uint32, bool) {
	if number < b.baseMsg {
		return 0, false
	}
	var rec [8]byte
	if _, err := b.jdx.ReadAt(rec[:], int64(number-b.baseMsg)*8); err != nil {
		return 0, false
	}
	off := binary.LittleEndian.Uint32(rec[4:8])
	if off == 0xFFFFFFFF || off < hdrInfo {
		return 0, false
	}
	return off, true
}

func (b *Base) touchInfo(activeDelta int) error {
	var info [24]byte
	if _, err := b.jhr.ReadAt(info[:], 0); err != nil {
		return err
	}
	mod := binary.LittleEndian.Uint32(info[8:12]) + 1
	active := int64(binary.LittleEndian.Uint32(info[12:16])) + int64(activeDelta)
	if active < 0 {
		active = 0
	}
	binary.LittleEndian.PutUint32(info[8:12], mod)
	binary.LittleEndian.PutUint32(info[12:16], uint32(active))
	_, err := b.jhr.WriteAt(info[:], 0)
	return err
}

func clip(s string, n int) string {
	b := []byte(s)
	if len(b) > n {
		b = b[:n]
	}
	return string(b)
}
