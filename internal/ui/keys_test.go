package ui

import "testing"

func TestParseArrows(t *testing.T) {
	ev, reply, rest := Parse([]byte("\x1b[A\x1b[6~\r"))
	if len(reply) != 0 || len(rest) != 0 {
		t.Fatalf("reply=%v rest=%v", reply, rest)
	}
	if len(ev) != 3 || ev[0].Kind != KindUp || ev[1].Kind != KindPgDn || ev[2].Kind != KindEnter {
		t.Fatalf("%+v", ev)
	}
	ev, _, _ = Parse([]byte("\x1b[D\x1b[C\x1bOD\x1bOC"))
	if len(ev) != 4 || ev[0].Kind != KindLeft || ev[1].Kind != KindRight || ev[2].Kind != KindLeft || ev[3].Kind != KindRight {
		t.Fatalf("%+v", ev)
	}
}

func TestParsePartialEsc(t *testing.T) {
	_, _, rest := Parse([]byte{0x1b})
	if len(rest) != 1 {
		t.Fatal(rest)
	}
	ev := Flush(rest)
	if len(ev) != 1 || ev[0].Kind != KindEsc {
		t.Fatal(ev)
	}
}

func TestParseTelnet(t *testing.T) {
	// IAC DO TERMINAL-TYPE, then 'q'
	ev, reply, rest := Parse([]byte{255, 253, 24, 'q'})
	if len(rest) != 0 || len(ev) != 1 || ev[0].Ch != 'q' {
		t.Fatalf("ev=%+v rest=%v", ev, rest)
	}
	if len(reply) != 3 || reply[0] != 255 || reply[1] != 252 || reply[2] != 24 {
		t.Fatalf("reply=%v", reply)
	}
	_, reply, _ = Parse([]byte{255, 253, 1})
	if string(reply) != string([]byte{255, 251, 1}) {
		t.Fatalf("echo reply=%v", reply)
	}
}
