package jam

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestPostReplyAndFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "general")
	writeFixture(t, path)
	b, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if !b.writable {
		t.Fatal("base was not opened for write")
	}
	n, err := b.Post(Outgoing{
		From:    "Martin",
		To:      "sysop@example.com",
		Subject: "Re: Welcome",
		Text:    []byte("Thanks\r\n"),
		Kind:    AreaEmail,
		ReplyTo: 1,
		Files:   []string{"notes.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Fatalf("number %d", n)
	}
	msgs, err := b.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("listed %d", len(msgs))
	}
	got := msgs[1]
	if got.To != "sysop@example.com" || got.Subject != "Re: Welcome" || len(got.Files) != 1 || got.Files[0] != "notes.txt" {
		t.Fatalf("%+v", got)
	}
	if got.Attr&attrFile == 0 || got.Attr&attrPrivate == 0 {
		t.Fatalf("attr %x", got.Attr)
	}
	text, err := os.ReadFile(path + ".JHR")
	if err != nil {
		t.Fatal(err)
	}
	// Original header starts at 1024. Reply1st is at +28.
	reply1 := text[1024+28 : 1024+32]
	if reply1[0] != 4 {
		t.Fatalf("reply link %v", reply1)
	}
}

func TestPostMsgID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "echo")
	writeFixture(t, path)
	b, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if _, err := b.Post(Outgoing{
		From: "Martin", To: "All", Subject: "Re: Test", Text: []byte("Hi\r"),
		Kind: AreaEcho, MsgID: "21:2/148 6ab8d746", ReplyID: "21:1/101 7b71e2df",
	}); err != nil {
		t.Fatal(err)
	}
	msgs, _ := b.List()
	got := msgs[len(msgs)-1]
	if got.MsgID != "21:2/148 6ab8d746" || b.MessageID(got) != got.MsgID {
		t.Fatalf("msgid %q", got.MsgID)
	}
	var fixed [fixedLen]byte
	if _, err := b.jhr.ReadAt(fixed[:], int64(got.HdrAt)); err != nil {
		t.Fatal(err)
	}
	if c := binary.LittleEndian.Uint32(fixed[16:]); c != CRC32String("21:2/148 6ab8d746") {
		t.Fatalf("msgid crc %08x", c)
	}
	if c := binary.LittleEndian.Uint32(fixed[20:]); c != CRC32String("21:1/101 7b71e2df") {
		t.Fatalf("reply crc %08x", c)
	}

	if _, err := b.Post(Outgoing{
		From: "Sysop", To: "All", Subject: "Kludge", Kind: AreaEcho,
		Text: []byte("\x01PID: EleBBS\r\x01MSGID: 21:2/148 00000001\rHello\r"),
	}); err != nil {
		t.Fatal(err)
	}
	msgs, _ = b.List()
	if id := b.MessageID(msgs[len(msgs)-1]); id != "21:2/148 00000001" {
		t.Fatalf("text msgid %q", id)
	}
}

func TestSeenAndDelete(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "general")
	writeFixture(t, path)
	b, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	msgs, err := b.List()
	if err != nil {
		t.Fatal(err)
	}
	h := msgs[0]
	high, attr, err := b.Seen(h, 7, false, "Martin")
	if err != nil {
		t.Fatal(err)
	}
	if high != 1 || attr&attrRead != 0 {
		t.Fatalf("high %d attr %x", high, attr)
	}
	high, attr, err = b.Seen(h, 7, true, "Martin")
	if err != nil {
		t.Fatal(err)
	}
	if high != 1 || attr&attrRead == 0 {
		t.Fatalf("received high %d attr %x", high, attr)
	}
	h.Number = 5
	high, _, err = b.Seen(h, 7, false, "Martin")
	if err != nil {
		t.Fatal(err)
	}
	if high != 5 || b.HighRead("Martin") != 5 || b.HighRead("Sysop") != 1 {
		t.Fatalf("martin %d sysop %d returned %d", b.HighRead("Martin"), b.HighRead("Sysop"), high)
	}
	// A record EleBBS wrote under another user number is still this user's.
	h.Number = 9
	if high, _, err = b.Seen(h, 3, false, "Martin"); err != nil || high != 9 {
		t.Fatalf("other user number high %d err %v", high, err)
	}
	if st, _ := b.jlr.Stat(); st.Size() != 32 {
		t.Fatalf("lastread file grew to %d bytes", st.Size())
	}
	if err := b.Delete(msgs[0]); err != nil {
		t.Fatal(err)
	}
	left, err := b.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Fatalf("listed %d after delete", len(left))
	}
}
