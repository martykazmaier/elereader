package ui

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"elereader/internal/door32"
	"elereader/internal/jam"
)

func TestReplyDefaults(t *testing.T) {
	if got := defaultTo(jam.AreaEcho, ""); got != "All" {
		t.Fatal(got)
	}
	if got := replySubject("Welcome"); got != "Re: Welcome" {
		t.Fatal(got)
	}
	if got := replySubject("Re: Welcome"); got != "Re: Welcome" {
		t.Fatal(got)
	}
	if got := validateReply(jam.AreaEmail, "sysop", "Hi", ""); got == "" {
		t.Fatal("email without @ was accepted")
	}
	if got := validateReply(jam.AreaEmail, "a@b.c", "Hi", ""); got != "" {
		t.Fatal(got)
	}
	if got := validateReply(jam.AreaNetmail, "All", "Hi", "1:2/3"); got == "" {
		t.Fatal("netmail to All was accepted")
	}
	if got := validateReply(jam.AreaNetmail, "Martin", "Hi", ""); got == "" {
		t.Fatal("netmail without an address was accepted")
	}
	if got := validateReply(jam.AreaNetmail, "Martin", "Hi", "1:2/3"); got != "" {
		t.Fatal(got)
	}
	if got := defaultNewTo(jam.AreaEcho); got != "All" {
		t.Fatal(got)
	}
	if got := defaultNewTo(jam.AreaNetmail); got != "" {
		t.Fatal(got)
	}
	if !replyPrivate(jam.AreaLocal, "Martin") || replyPrivate(jam.AreaLocal, "All") {
		t.Fatal("local privacy")
	}
}

func TestEditorCommand(t *testing.T) {
	got, err := editorArgs(`c:\bbs\editor.exe`, "1", "38400", "45", "60")
	if err != nil || got != `c:\bbs\editor.exe 1 38400 45 60` {
		t.Fatal(got, err)
	}
	if _, err := editorArgs(" ", "1", "0", "1440", "60"); err == nil {
		t.Fatal("empty editor was accepted")
	}
}

func TestSubjectAsksUpload(t *testing.T) {
	var buf bytes.Buffer
	a := &App{
		scr:       &Screen{w: &buf},
		user:      door32.Drop{Emulation: door32.EmuANSI, Started: time.Now()},
		areas:     []Area{{Name: "General"}},
		mode:      modeReply,
		replyTo:   "All",
		replyFld:  1,
		replyEdit: true,
		replyBuf:  "Hello",
	}
	a.onReply(Event{Kind: KindEnter})
	if a.mode != modeUpAsk {
		t.Fatalf("mode %d", a.mode)
	}
	a.onUpAsk(Event{Kind: KindEsc})
	if a.mode != modeReply {
		t.Fatalf("Esc went to mode %d", a.mode)
	}
}

func TestNewMsgID(t *testing.T) {
	now := time.Unix(0x6ab8d746, 0)
	if got := newMsgID("21:2/148", 0, now); got != "21:2/148 6ab8d746" {
		t.Fatal(got)
	}
	if newMsgID("21:2/148", 1, now) == newMsgID("21:2/148", 2, now) {
		t.Fatal("nodes share a serial")
	}
	if got := newMsgID(" ", 1, now); got != "" {
		t.Fatal(got)
	}
}

func TestUploadedText(t *testing.T) {
	dir := t.TempDir()
	small := filepath.Join(dir, "a.txt")
	big := filepath.Join(dir, "b.txt")
	_ = os.WriteFile(small, []byte("hi"), 0644)
	_ = os.WriteFile(big, []byte("Hello there\r\n\x1a\x1a\x1a"), 0644)
	if got := string(uploadedText([]string{small, big})); got != "Hello there\r\n" {
		t.Fatalf("%q", got)
	}
}

func TestQuoteKeepsBreaks(t *testing.T) {
	got := quoteLines([]string{"", "Hi there.", "", "", "Second para", "line two", ""})
	want := []string{"> Hi there.", "", "> Second para", "> line two"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("%q", got)
	}
}

func TestMsgInf(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "msginf")
	if err := writeMsgInf(path, "Martin", "All", "Hello", "12", "General", false); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "Martin\r\nAll\r\nHello\r\n12\r\nGeneral\r\nNO\r\n" {
		t.Fatalf("%q", b)
	}
}
