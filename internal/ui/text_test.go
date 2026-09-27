package ui

import (
	"strings"
	"testing"
)

func TestBodyLines(t *testing.T) {
	raw := []byte("\x01MSGID: 1:2/3 1234\r\nHello \x1b[1;33mthere\x1b[0m\r\nSEEN-BY: 1/2\r\n")
	lines := bodyLines(raw, 78)
	if len(lines) != 1 {
		t.Fatalf("%q", lines)
	}
	plain := strings.TrimRight(string(stripSGR([]byte(lines[0]))), " ")
	if plain != "Hello there" {
		t.Fatalf("%q", plain)
	}
	if !strings.Contains(lines[0], "\x1b[0;1;33;40m") {
		t.Fatalf("color missing %q", lines[0])
	}
}

func TestANSICursor(t *testing.T) {
	lines := bodyLines([]byte("\x1b[2J\x1b[1;1HA\x1b[1;3HB"), 80)
	if len(lines) != 1 {
		t.Fatalf("%q", lines)
	}
	plain := string(stripSGR([]byte(lines[0])))
	if !strings.HasPrefix(plain, "A B") {
		t.Fatalf("%q", plain)
	}
	if strings.Contains(lines[0], "[2J") || strings.Contains(lines[0], "[1;") {
		t.Fatalf("cursor code leaked %q", lines[0])
	}
}

func TestSoftCR(t *testing.T) {
	lines := bodyLines([]byte("Hello\x8dworld"), 80)
	if len(lines) != 2 {
		t.Fatalf("%q", lines)
	}
	if strings.TrimRight(string(stripSGR([]byte(lines[0]))), " ") != "Hello" {
		t.Fatalf("%q", lines[0])
	}
	if strings.TrimRight(string(stripSGR([]byte(lines[1]))), " ") != "world" {
		t.Fatalf("%q", lines[1])
	}
}

func TestWrap(t *testing.T) {
	got := wrapVisible([]byte("abcdefghij"), 4)
	if len(got) != 3 || got[0] != "abcd" || got[1] != "efgh" || got[2] != "ij" {
		t.Fatalf("%q", got)
	}
}

func TestSlide(t *testing.T) {
	sel, top := slide(0, 0, 12, 40, 12)
	if sel != 12 || top != 1 {
		t.Fatalf("sel=%d top=%d", sel, top)
	}
	sel, top = slide(5, 0, 12, 40, 5)
	if sel != 5 || top != 0 {
		t.Fatalf("sel=%d top=%d", sel, top)
	}
	sel, top = slide(0, 0, 12, 0, 3)
	if sel != 0 || top != 0 {
		t.Fatalf("empty sel=%d top=%d", sel, top)
	}
}
