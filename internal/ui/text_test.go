package ui

import (
	"strings"
	"testing"
)

func TestBodyLines(t *testing.T) {
	raw := []byte("\x01MSGID: 1:2/3 1234\r\nHello \x1b[1;33mthere\x1b[0m\r\nSEEN-BY: 1/2\r\n")
	lines := bodyLines(raw, 80)
	if len(lines) != 1 {
		t.Fatalf("%q", lines)
	}
	plain := strings.TrimRight(string(stripSGR([]byte(lines[0]))), " ")
	if plain != "Hello there" {
		t.Fatalf("%q", plain)
	}
	if !strings.Contains(lines[0], "33") {
		t.Fatalf("color missing %q", lines[0])
	}
}

func TestCursorReturnsToSameRow(t *testing.T) {
	raw := []byte("LEFT\r\n\x1b[A\x1b[10CRIGHT")
	lines := bodyLines(raw, 80)
	if len(lines) < 1 {
		t.Fatal("no lines")
	}
	plain := string(stripSGR([]byte(lines[0])))
	if !strings.HasPrefix(plain, "LEFT") || len(plain) < 15 || plain[10:15] != "RIGHT" {
		t.Fatalf("%q", plain)
	}
}

func TestBareCRIsANewLine(t *testing.T) {
	raw := []byte("one\rtwo\r * Origin: board\r")
	lines := bodyLines(raw, 80)
	if len(lines) != 3 {
		t.Fatalf("%d %q", len(lines), lines)
	}
	if strings.TrimRight(string(stripSGR([]byte(lines[0]))), " ") != "one" {
		t.Fatalf("%q", lines[0])
	}
	if strings.TrimRight(string(stripSGR([]byte(lines[2]))), " ") != " * Origin: board" {
		t.Fatalf("%q", lines[2])
	}
}

func TestBBSColorCodes(t *testing.T) {
	lines := bodyLines([]byte("@X1EYellow on blue\r|15Bright\r`A14:`F14:Radu\r"), 80)
	if len(lines) < 3 {
		t.Fatalf("%q", lines)
	}
	if !strings.Contains(lines[0], "33") || !strings.Contains(lines[0], "44") {
		t.Fatalf("at-X want yellow on blue, got %q", lines[0])
	}
	if strings.TrimRight(string(stripSGR([]byte(lines[0]))), " ") != "Yellow on blue" {
		t.Fatalf("plain %q", lines[0])
	}
	if !strings.Contains(lines[1], "1;") {
		t.Fatalf("pipe %q", lines[1])
	}
	if strings.TrimRight(string(stripSGR([]byte(lines[2]))), " ") != "Radu" {
		t.Fatalf("radu %q", lines[2])
	}
	raw := expandBBSColors([]byte("|07ok|xx|@X0Fhi"))
	if !strings.Contains(string(raw), "ok") || !strings.Contains(string(raw), "|xx") {
		t.Fatalf("literal pipe %q", raw)
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
