package ui

import (
	"bytes"
	"testing"
	"time"

	"elereader/internal/door32"
	"elereader/internal/jam"
)

func TestPaintLightbar(t *testing.T) {
	var buf bytes.Buffer
	when := time.Date(2026, 9, 27, 1, 0, 0, 0, time.UTC)
	a := &App{
		scr: &Screen{w: &buf},
		user: door32.Drop{
			Alias:     "Martin",
			RealName:  "Martin",
			Emulation: door32.EmuANSI,
			Node:      1,
			Started:   time.Now(),
		},
		areas: []Area{{Name: "General"}},
		mode:  modeList,
		msgs: []jam.Header{
			{Number: 1, From: "Sysop", To: "All", Subject: "Welcome", When: when},
			{Number: 2, From: "Martin", To: "All", Subject: "Lightbar", When: when},
		},
		sel:  0,
		top:  0,
		high: 0,
	}
	a.paintAll()
	if !bytes.Contains(buf.Bytes(), []byte{chTL}) {
		t.Fatal("missing frame")
	}
	if !bytes.Contains(buf.Bytes(), []byte(attrBar)) {
		t.Fatal("missing lightbar")
	}
	if !bytes.Contains(buf.Bytes(), []byte("Welcome")) || !bytes.Contains(buf.Bytes(), []byte("Lightbar")) {
		t.Fatal(buf.String())
	}

	buf.Reset()
	a.move(1)
	out := buf.String()
	if !bytes.Contains(buf.Bytes(), []byte(attrBar)) || !bytes.Contains(buf.Bytes(), []byte("Lightbar")) {
		t.Fatal(out)
	}
	if bytes.Contains(buf.Bytes(), []byte("Elereader")) {
		t.Fatal("bar move redrew the title")
	}
}
