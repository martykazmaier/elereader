package jam

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestReadBase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "general")
	writeFixture(t, path)

	b, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	if got := b.HighRead("Sysop"); got != 1 {
		t.Fatalf("high read %d", got)
	}
	msgs, err := b.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want the one live header", len(msgs))
	}
	m := msgs[0]
	if m.Number != 1 || m.From != "Sysop" || m.To != "All" || m.Subject != "Welcome" {
		t.Fatalf("%+v", m)
	}
	if m.Deleted() {
		t.Fatal("live message marked deleted")
	}
	text, err := b.Text(m)
	if err != nil {
		t.Fatal(err)
	}
	if string(text) != "Welcome aboard\r\n" {
		t.Fatalf("%q", text)
	}
}

func writeFixture(t *testing.T, path string) {
	t.Helper()
	subs := append(field(2, "Sysop"), field(3, "All")...)
	subs = append(subs, field(6, "Welcome")...)
	text := []byte("Welcome aboard\r\n")

	jhr := make([]byte, 1024+76+len(subs))
	binary.LittleEndian.PutUint32(jhr[0:], sigJAM)
	binary.LittleEndian.PutUint32(jhr[12:], 1) // active
	binary.LittleEndian.PutUint32(jhr[16:], 0xFFFFFFFF)
	binary.LittleEndian.PutUint32(jhr[20:], 1) // base message number

	off := 1024
	binary.LittleEndian.PutUint32(jhr[off:], sigJAM)
	binary.LittleEndian.PutUint16(jhr[off+4:], 1) // revision
	binary.LittleEndian.PutUint32(jhr[off+8:], uint32(len(subs)))
	binary.LittleEndian.PutUint32(jhr[off+36:], 1_700_000_000) // written
	binary.LittleEndian.PutUint32(jhr[off+48:], 1)             // message number
	binary.LittleEndian.PutUint32(jhr[off+52:], attrTypeLoc)
	binary.LittleEndian.PutUint32(jhr[off+60:], 0) // text offset
	binary.LittleEndian.PutUint32(jhr[off+64:], uint32(len(text)))
	copy(jhr[off+76:], subs)

	// Second header is deleted and must not be listed. It lives after the first.
	delSubs := field(2, "Gone")
	delAt := len(jhr)
	extra := make([]byte, 76+len(delSubs))
	binary.LittleEndian.PutUint32(extra[0:], sigJAM)
	binary.LittleEndian.PutUint16(extra[4:], 1)
	binary.LittleEndian.PutUint32(extra[8:], uint32(len(delSubs)))
	binary.LittleEndian.PutUint32(extra[52:], attrDeleted)
	copy(extra[76:], delSubs)
	jhr = append(jhr, extra...)

	if err := os.WriteFile(path+".JHR", jhr, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".JDT", text, 0644); err != nil {
		t.Fatal(err)
	}

	idx := make([]byte, 24)
	binary.LittleEndian.PutUint32(idx[0:], CRC32String("All"))
	binary.LittleEndian.PutUint32(idx[4:], uint32(off))
	binary.LittleEndian.PutUint32(idx[8:], 0xFFFFFFFF) // empty slot
	binary.LittleEndian.PutUint32(idx[12:], 0xFFFFFFFF)
	binary.LittleEndian.PutUint32(idx[16:], CRC32String("All"))
	binary.LittleEndian.PutUint32(idx[20:], uint32(delAt))
	if err := os.WriteFile(path+".JDX", idx, 0644); err != nil {
		t.Fatal(err)
	}

	jlr := make([]byte, 16)
	binary.LittleEndian.PutUint32(jlr[0:], CRC32String("Sysop"))
	binary.LittleEndian.PutUint32(jlr[4:], 1)
	binary.LittleEndian.PutUint32(jlr[8:], 1)
	binary.LittleEndian.PutUint32(jlr[12:], 1)
	if err := os.WriteFile(path+".JLR", jlr, 0644); err != nil {
		t.Fatal(err)
	}
}
