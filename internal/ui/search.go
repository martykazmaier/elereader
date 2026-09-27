package ui

import (
	"strings"

	"elereader/internal/jam"
)

var searchLabels = map[byte]string{
	's': "Subject",
	't': "To",
	'f': "From",
	'b': "Body",
}

func (a *App) beginSearch() {
	if len(a.msgs) == 0 {
		a.note = "No messages to search."
		a.paintAll()
		return
	}
	a.searchFrom = a.mode
	a.searchField = 0
	a.mode = modeSearch
	a.paintAll()
}

func (a *App) onSearch(ev Event) bool {
	if a.searchField == 0 {
		switch ev.Kind {
		case KindEsc:
			a.endSearch()
		case KindByte:
			c := ev.Ch | 0x20
			if c == 'k' {
				c = 'b'
			}
			if _, ok := searchLabels[c]; ok {
				a.searchField = c
				a.searchBuf = a.searchText
				a.paintAll()
			} else if c == 'q' {
				a.endSearch()
			}
		}
		return false
	}
	switch ev.Kind {
	case KindEsc:
		a.endSearch()
	case KindEnter:
		a.searchText = strings.TrimSpace(a.searchBuf)
		a.runSearch()
	case KindByte:
		if ev.Ch == 8 || ev.Ch == 127 {
			if len(a.searchBuf) > 0 {
				a.searchBuf = a.searchBuf[:len(a.searchBuf)-1]
				a.paintSearchPrompt()
			}
			return false
		}
		if ev.Ch >= 32 && len(a.searchBuf) < 50 {
			a.searchBuf += string(ev.Ch)
			a.paintSearchPrompt()
		}
	}
	return false
}

func (a *App) endSearch() {
	a.mode = a.searchFrom
	a.searchField = 0
	a.paintAll()
}

// runSearch moves to the next message after the current one that holds a
// partial, case-insensitive match, wrapping to the top of the area.
func (a *App) runSearch() {
	want := strings.ToLower(a.searchText)
	field := a.searchField
	a.searchField = 0
	a.mode = a.searchFrom
	if want == "" {
		a.paintAll()
		return
	}
	n := len(a.msgs)
	for step := 1; step <= n; step++ {
		i := (a.sel + step) % n
		if a.searchMatch(a.msgs[i], field, want) {
			a.sel = i
			if a.sel < a.top || a.sel >= a.top+listRows {
				a.top = a.sel - listRows/2
				if a.top < 0 {
					a.top = 0
				}
			}
			if a.mode == modeRead {
				a.loadBody()
			}
			a.note = "Found in message " + itoa(a.msgs[i].Number)
			a.paintAll()
			return
		}
	}
	a.note = "No " + searchLabels[field] + " matches \"" + a.searchText + "\"."
	a.paintAll()
}

func (a *App) searchMatch(h jam.Header, field byte, want string) bool {
	if !a.canSee(h) {
		return false
	}
	var have string
	switch field {
	case 's':
		have = a.listSubject(h) + " " + h.Subject
	case 't':
		have = h.To
	case 'f':
		have = h.From
	case 'b':
		if a.base == nil {
			return false
		}
		raw, err := a.base.Text(h)
		if err != nil {
			return false
		}
		have = string(stripSGR(filterMessage(raw)))
	}
	return strings.Contains(strings.ToLower(have), want)
}

func (a *App) paintSearch() {
	a.mode = a.searchFrom
	a.paintAll()
	a.mode = modeSearch
	a.paintSearchPrompt()
}

func (a *App) paintSearchPrompt() {
	if a.searchField == 0 {
		a.scr.content(22, attrBar, fit(" Search by:  S Subject  T To  F From  B Body keyword", contentWidth))
		a.scr.rule(24, chBL, chBR, "S T F B  Esc Cancel")
		return
	}
	a.scr.content(22, attrBar, fit(" "+searchLabels[a.searchField]+" contains: "+a.searchBuf+"_", contentWidth))
	a.scr.rule(24, chBL, chBR, "Enter Search  Esc Cancel")
}
