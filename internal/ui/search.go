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
				a.searchBuf = ""
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

// runSearch lists every message in the area with a partial,
// case-insensitive match. Esc or Q on that list brings back the full one.
func (a *App) runSearch() {
	want := strings.ToLower(a.searchText)
	field := a.searchField
	a.searchField = 0
	a.mode = a.searchFrom
	if want == "" {
		a.paintAll()
		return
	}
	pool := a.msgs
	if a.allMsgs != nil {
		pool = a.allMsgs
	}
	var hits []jam.Header
	for _, m := range pool {
		if a.searchMatch(m, field, want) {
			hits = append(hits, m)
		}
	}
	if len(hits) == 0 {
		a.note = "No " + searchLabels[field] + " matches \"" + a.searchText + "\"."
		a.paintAll()
		return
	}
	if a.allMsgs == nil {
		a.allMsgs = a.msgs
	}
	a.msgs = hits
	a.sel, a.top = 0, 0
	a.searchTitle = searchLabels[field] + " \"" + a.searchText + "\""
	a.note = itoa(uint32(len(hits))) + " found"
	a.mode = modeList
	a.paintAll()
}

// clearSearch goes back to the full list, keeping the message the
// lightbar was on.
func (a *App) clearSearch() {
	cur := uint32(0)
	if a.sel >= 0 && a.sel < len(a.msgs) {
		cur = a.msgs[a.sel].Number
	}
	a.msgs = a.allMsgs
	a.allMsgs = nil
	a.searchTitle = ""
	a.sel = 0
	for i, m := range a.msgs {
		if m.Number == cur {
			a.sel = i
			break
		}
	}
	a.top = 0
	if a.sel >= listRows {
		a.top = a.sel - listRows + 1
	}
	a.paintAll()
}

// dropSearch forgets search results when the list is reloaded.
func (a *App) dropSearch() {
	a.allMsgs = nil
	a.searchTitle = ""
}

func (a *App) setAttr(number, attr uint32) {
	for _, list := range [][]jam.Header{a.msgs, a.allMsgs} {
		for i := range list {
			if list[i].Number == number {
				list[i].Attr = attr
			}
		}
	}
}

func (a *App) forgetMessage(number uint32) {
	for i, m := range a.allMsgs {
		if m.Number == number {
			a.allMsgs = append(a.allMsgs[:i], a.allMsgs[i+1:]...)
			return
		}
	}
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
