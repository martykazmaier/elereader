package ui

import (
	"os"
	"path/filepath"
	"testing"

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
