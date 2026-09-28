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

func TestSearch(t *testing.T) {
	var buf bytes.Buffer
	a := &App{
		scr:   &Screen{w: &buf},
		user:  door32.Drop{Emulation: door32.EmuANSI, Started: time.Now()},
		areas: []Area{{Name: "General"}},
		mode:  modeList,
		msgs: []jam.Header{
			{Number: 1, From: "Sysop", To: "All", Subject: "Welcome"},
			{Number: 2, From: "Martin", To: "All", Subject: "Lightbar"},
			{Number: 3, From: "Avon", To: "Martin", Subject: "Crash test"},
		},
		sel: 1,
	}
	keys := func(s string) {
		for i := 0; i < len(s); i++ {
			a.on(Event{Kind: KindByte, Ch: s[i]})
		}
	}
	keys("sfsys")
	a.on(Event{Kind: KindEnter})
	if a.mode != modeList || len(a.msgs) != 1 || a.msgs[0].Number != 1 {
		t.Fatalf("from search: mode %d msgs %v", a.mode, a.msgs)
	}
	a.on(Event{Kind: KindEsc})
	if len(a.msgs) != 3 || a.allMsgs != nil || a.sel != 0 {
		t.Fatalf("after Esc: %d msgs, sel %d", len(a.msgs), a.sel)
	}
	keys("ssTEST")
	a.on(Event{Kind: KindEnter})
	if len(a.msgs) != 1 || a.msgs[0].Number != 3 {
		t.Fatalf("subject search %v", a.msgs)
	}
	keys("q")
	if len(a.msgs) != 3 || a.sel != 2 {
		t.Fatalf("after Q: %d msgs, sel %d", len(a.msgs), a.sel)
	}
	keys("stnobody")
	a.on(Event{Kind: KindEnter})
	if len(a.msgs) != 3 || a.allMsgs != nil || a.note == "" {
		t.Fatalf("miss: %d msgs note %q", len(a.msgs), a.note)
	}
	keys("?")
	if a.mode != modeHelp {
		t.Fatalf("help mode %d", a.mode)
	}
	keys("x")
	if a.mode != modeList {
		t.Fatalf("after help mode %d", a.mode)
	}
}

func TestUnreadMark(t *testing.T) {
	a := &App{
		user:  door32.Drop{RealName: "Martin", Alias: "Marty"},
		areas: []Area{{Kind: jam.AreaEmail}},
		high:  3,
	}
	if !a.unread(jam.Header{Number: 8, To: "Martin", Attr: 0x4}) {
		t.Fatal("new mail")
	}
	if a.unread(jam.Header{Number: 8, To: "Marty", Attr: 0x4 | 0x8}) {
		t.Fatal("received mail is read even past the high water")
	}
	if a.unread(jam.Header{Number: 3, To: "All"}) {
		t.Fatal("at the high water is read")
	}
	if !a.unread(jam.Header{Number: 4, To: "All"}) {
		t.Fatal("past the high water is new")
	}
	if !a.unread(jam.Header{Number: 2, To: "Martin", Attr: 0x4}) {
		t.Fatal("private mail to me is new until received, even below the high water")
	}
	other := jam.Header{Number: 9, From: "Sue", To: "Bob", Attr: 0x4}
	if a.unread(other) || a.mark(other) != "+" {
		t.Fatalf("someone else's private mail: unread %v mark %q", a.unread(other), a.mark(other))
	}
	mine := jam.Header{Number: 9, From: "Martin", To: "Bob", Attr: 0x4}
	if a.unread(mine) || a.mark(mine) != " " {
		t.Fatalf("mail I sent: unread %v mark %q", a.unread(mine), a.mark(mine))
	}
}

func TestPrivateVisibility(t *testing.T) {
	a := &App{
		user:  door32.Drop{RealName: "Martin", Alias: "Marty"},
		areas: []Area{{Kind: jam.AreaLocal}},
	}
	msgs := []jam.Header{
		{Number: 1, From: "Sue", To: "Bob", Attr: 0x4},
		{Number: 2, From: "Sue", To: "Marty", Attr: 0x4},
		{Number: 3, From: "Martin", To: "Bob", Attr: 0x4},
		{Number: 4, From: "Sue", To: "All", Attr: 0x4},
		{Number: 5, From: "Sue", To: "Bob"},
	}
	got := a.visible(append([]jam.Header(nil), msgs...))
	if len(got) != 4 || got[0].Number != 2 {
		t.Fatalf("user sees %+v", got)
	}
	a.areas[0].SysopAccess = true
	if got := a.visible(append([]jam.Header(nil), msgs...)); len(got) != 5 {
		t.Fatalf("sysop sees %d", len(got))
	}
}
