package ui

// Kind is a decoded caller key.
type Kind int

const (
	KindNone Kind = iota
	KindUp
	KindDown
	KindPgUp
	KindPgDn
	KindHome
	KindEnd
	KindEnter
	KindEsc
	KindLeft
	KindRight
	KindByte
)

// Event is one key or a literal byte.
type Event struct {
	Kind Kind
	Ch   byte
}

const (
	iac  = 255
	dont = 254
	do   = 253
	wont = 252
	will = 251
	sb   = 250
	se   = 240
	echo = 1
)

// WillEcho tells the caller this door paints the screen, so the terminal
// must not print keystrokes on its own.
func WillEcho() []byte {
	return []byte{iac, will, echo}
}

func telnetAnswer(cmd, opt byte) []byte {
	switch cmd {
	case will:
		return []byte{iac, dont, opt}
	case do:
		if opt == echo {
			return []byte{iac, will, echo}
		}
		return []byte{iac, wont, opt}
	default:
		return nil
	}
}

// Parse pulls complete events out of b.
// enterTail drops the LF or NUL that telnet sends after the CR of Enter.
// Bytes are read one at a time, so the pair can be split.
type enterTail struct {
	afterCR bool
}

func (t *enterTail) skip(b byte) bool {
	after := t.afterCR
	t.afterCR = b == '\r'
	return after && (b == '\n' || b == 0)
}

// reply is telnet IAC traffic the door should send back.
// rest is an incomplete suffix; wait briefly, then Flush it.
func Parse(b []byte) (events []Event, reply []byte, rest []byte) {
	i := 0
	for i < len(b) {
		c := b[i]
		if c == iac {
			if i+1 >= len(b) {
				return events, reply, b[i:]
			}
			cmd := b[i+1]
			switch cmd {
			case iac:
				events = append(events, Event{Kind: KindByte, Ch: iac})
				i += 2
			case will, wont, do, dont:
				if i+2 >= len(b) {
					return events, reply, b[i:]
				}
				opt := b[i+2]
				reply = append(reply, telnetAnswer(cmd, opt)...)
				i += 3
			case sb:
				j := i + 2
				for j+1 < len(b) {
					if b[j] == iac && b[j+1] == se {
						j += 2
						break
					}
					j++
				}
				if j+1 >= len(b) && (j >= len(b) || b[len(b)-1] != se) {
					// still inside a subnegotiation, unless we closed it
					closed := j >= 2 && j <= len(b) && b[j-2] == iac && b[j-1] == se
					if !closed {
						return events, reply, b[i:]
					}
				}
				i = j
			default:
				i += 2
			}
			continue
		}
		if c == 0x1b {
			ev, n, ok := parseEsc(b[i:])
			if !ok {
				return events, reply, b[i:]
			}
			if ev.Kind != KindNone {
				events = append(events, ev)
			}
			i += n
			continue
		}
		if c == '\r' {
			events = append(events, Event{Kind: KindEnter})
			i++
			if i < len(b) && b[i] == '\n' {
				i++
			}
			continue
		}
		if c == '\n' {
			events = append(events, Event{Kind: KindEnter})
			i++
			continue
		}
		if c == 3 {
			events = append(events, Event{Kind: KindEsc})
			i++
			continue
		}
		if c == 0 || c == 0x0c {
			i++
			continue
		}
		events = append(events, Event{Kind: KindByte, Ch: c})
		i++
	}
	return events, reply, nil
}

// Flush turns a timed-out partial escape into Esc. Other leftovers are dropped.
func Flush(rest []byte) []Event {
	if len(rest) == 1 && rest[0] == 0x1b {
		return []Event{{Kind: KindEsc}}
	}
	return nil
}

func parseEsc(b []byte) (Event, int, bool) {
	if len(b) < 2 {
		return Event{}, 0, false
	}
	if b[1] == '[' {
		return parseCSI(b)
	}
	if b[1] == 'O' {
		if len(b) < 3 {
			return Event{}, 0, false
		}
		return Event{Kind: ss3Kind(b[2])}, 3, true
	}
	return Event{Kind: KindEsc}, 1, true
}

func parseCSI(b []byte) (Event, int, bool) {
	i := 2
	for i < len(b) {
		if b[i] >= 0x40 && b[i] <= 0x7e {
			final := b[i]
			body := string(b[2:i])
			return Event{Kind: csiKind(final, body)}, i + 1, true
		}
		i++
	}
	return Event{}, 0, false
}

func ss3Kind(b byte) Kind {
	switch b {
	case 'A':
		return KindUp
	case 'B':
		return KindDown
	case 'C':
		return KindRight
	case 'D':
		return KindLeft
	case 'H':
		return KindHome
	case 'F':
		return KindEnd
	default:
		return KindNone
	}
}

func csiKind(final byte, body string) Kind {
	switch final {
	case 'A':
		return KindUp
	case 'B':
		return KindDown
	case 'C':
		return KindRight
	case 'D':
		return KindLeft
	case 'H':
		return KindHome
	case 'F':
		return KindEnd
	case '~':
		p := body
		if i := indexByteStr(p, ';'); i >= 0 {
			p = p[:i]
		}
		switch p {
		case "1", "7":
			return KindHome
		case "4", "8":
			return KindEnd
		case "5":
			return KindPgUp
		case "6":
			return KindPgDn
		default:
			return KindNone
		}
	default:
		return KindNone
	}
}

func indexByteStr(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}
