package ui

var helpLines = []string{
	"",
	" Message list",
	"   Up/Down  Home/End            Move the lightbar",
	"   Left/Right  PgUp/PgDn        Previous or next page",
	"   Enter                        Read the message",
	"   P                            Post a new message",
	"   R                            Reply to the message",
	"   K                            Kill (delete) the message",
	"   S                            Search by Subject, To, From or Body",
	"   Q or Esc                     Quit, or leave the search results",
	"",
	" Reading a message",
	"   Up/Down PgUp/PgDn Space      Scroll the message",
	"   Left/Right  N/B  +/-         Next or previous message",
	"   R  P  K  S                   Reply, Post, Kill, Search",
	"   D                            Download this message as text",
	"   F                            Download the attached files",
	"   Q or Esc                     Back to the list",
	"",
	" Marks:  * new to you   + someone else's private mail",
}

func (a *App) beginHelp() {
	a.helpFrom = a.mode
	a.mode = modeHelp
	a.paintAll()
}

func (a *App) onHelp(ev Event) bool {
	a.mode = a.helpFrom
	a.paintAll()
	return false
}

func (a *App) paintHelp() {
	a.scr.rule(1, chTL, chTR, "Elereader help")
	for y := 2; y <= 22; y++ {
		text := ""
		if i := y - 2; i < len(helpLines) {
			text = helpLines[i]
		}
		color := attrNorm
		if len(text) > 1 && text[1] != ' ' {
			color = attrHead
		}
		a.scr.content(y, color, fit(text, contentWidth))
	}
	a.scr.content(statusY, attrNorm, a.statusText())
	a.scr.rule(24, chBL, chBR, "Any key to return")
}
