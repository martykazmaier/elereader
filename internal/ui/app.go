package ui

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"elereader/internal/door32"
	"elereader/internal/jam"
)

const (
	listY      = 3
	listRows   = 19
	bodyY      = 6
	bodyRows   = 16
	statusY    = 23
	modeList   = 1
	modeRead   = 2
	modeReply  = 3
	modeProto  = 4
	modeAsk    = 5
	modeUpAsk  = 6
	modeSearch = 7
	modeHelp   = 8
)

// Area is the one conference EleBBS already selected.
type Area struct {
	Name        string
	Path        string
	Kind        jam.AreaKind
	Origin      string
	Editor      string
	Attach      string
	AllowAttach bool
	SysopAccess bool
	Node        string
	Sys         string
	Fetch       string
}

// App is the full-screen lightbar reader.
type App struct {
	port        io.ReadWriter
	scr         *Screen
	user        door32.Drop
	areas       []Area
	area        int
	base        *jam.Base
	msgs        []jam.Header
	high        uint32
	sel         int
	top         int
	mode        int
	err         string
	body        []string
	bodyTop     int
	quit        bool
	replyTo     string
	replySub    string
	replyFld    int
	replyEdit   bool
	replyBuf    string
	replyHold   string
	replyNote   string
	replyAddr   string
	replyFrom   int
	postNew     bool
	quote       []string
	editPath    string
	attaches    []string
	note        string
	protos      []Protocol
	protoSel    int
	protoTop    int
	protoFiles  []string
	protoUp     bool
	protoMsg    bool
	uploadDir   string
	searchFrom  int
	searchField byte
	searchBuf   string
	searchText  string
	helpFrom    int
	allMsgs     []jam.Header
	searchTitle string
	release     func() func()
	settle      time.Time
	redraw      chan struct{}
}

// Terminals keep their transfer window up for a moment after the protocol
// program exits, so a paint sent right away can be lost. The screen is sent
// again after these delays, and protocol leftovers are dropped until the first.
var redrawAfter = []time.Duration{time.Second, 3 * time.Second}

// setBlocking asks the caller port to wait inside a transfer program.
// Ports that do not support it keep their normal reads.
func setBlocking(p io.ReadWriter, on bool) {
	type blocker interface {
		Blocking(bool)
	}
	if p == nil {
		return
	}
	if b, ok := p.(blocker); ok {
		b.Blocking(on)
	}
}

// lendCaller stops the reader and lets an external program use the connection.
// Call the returned function after the message screen has been drawn again.
// lendCaller hands the line to an external program. After a transfer,
// protocol bytes can trail in, so keys are ignored briefly; after the
// editor the caller's next key is real.
func (a *App) lendCaller(transfer bool) func() {
	var resume func()
	if a.release != nil {
		resume = a.release()
	}
	setBlocking(a.port, true)
	if a.scr != nil {
		a.scr.text("\x1b[0m\x1b[?25h\x1b[2J\x1b[1;1H")
	}
	return func() {
		setBlocking(a.port, false)
		if resume != nil {
			resume()
		}
		a.redrawSoon(transfer)
	}
}

func (a *App) redrawSoon(transfer bool) {
	if a.redraw == nil || len(redrawAfter) == 0 {
		return
	}
	if transfer {
		a.settle = time.Now().Add(redrawAfter[0])
	}
	for _, d := range redrawAfter {
		time.AfterFunc(d, func() {
			select {
			case a.redraw <- struct{}{}:
			default:
			}
		})
	}
}

// Run draws the reader and handles keys until the caller quits,
// the carrier drops, or the DOOR32 time limit runs out.
// areas holds the one conference EleBBS already selected. fail is
// shown when that conference could not be resolved.
func Run(p io.ReadWriter, user door32.Drop, areas []Area, fail string) {
	a := &App{
		port:  p,
		scr:   &Screen{w: p},
		user:  user,
		areas: areas,
		mode:  modeList,
	}
	defer func() {
		if a.base != nil {
			a.base.Close()
		}
		out := "\x1b[0m\x1b[?7h\x1b[?25h\x1b[2J\x1b[H"
		if a.quit && a.timeUp() {
			out = "\x1b[0m\x1b[?7h\x1b[?25h\x1b[2J\x1b[12;1HTime is up.\r\n"
		}
		_, _ = p.Write([]byte(out))
	}()

	if len(areas) == 0 {
		a.err = fail
		if a.err == "" {
			a.err = "No message area. EXITINFO.BBS is in the node directory. MESSAGES.RA is in ELEBBS or RA."
		}
	} else {
		a.openArea(0)
	}
	if a.user.CommType == door32.CommTelnet {
		_, _ = p.Write(WillEcho())
	}
	a.paintAll()

	keys := make(chan byte, 512)
	gone := make(chan struct{})
	a.redraw = make(chan struct{}, 1)
	var mu sync.Mutex
	cond := sync.NewCond(&mu)
	paused := false
	idle := false
	a.release = func() func() {
		mu.Lock()
		paused = true
		for !idle {
			cond.Wait()
		}
		mu.Unlock()
		return func() {
			mu.Lock()
			paused = false
			cond.Broadcast()
			mu.Unlock()
		}
	}
	go func() {
		defer close(gone)
		buf := make([]byte, 256)
		for {
			mu.Lock()
			for paused {
				idle = true
				cond.Broadcast()
				for paused {
					cond.Wait()
				}
				idle = false
			}
			mu.Unlock()
			n, err := p.Read(buf)
			if n > 0 {
				for i := 0; i < n; i++ {
					select {
					case keys <- buf[i]:
					default:
					}
				}
			}
			if errors.Is(err, io.EOF) {
				return
			}
			if err != nil {
				time.Sleep(15 * time.Millisecond)
				continue
			}
		}
	}()

	var pending []byte
	var tail enterTail
	esc := time.NewTimer(time.Hour)
	stopTimer(esc)
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()

	for {
		select {
		case <-gone:
			return
		case <-tick.C:
			if a.timeUp() {
				a.quit = true
				return
			}
			if a.mode != modeRead {
				a.paintStatus()
			}
		case <-a.redraw:
			pending = nil
			stopTimer(esc)
			a.paintAll()
		case b := <-keys:
			if time.Now().Before(a.settle) {
				continue
			}
			if tail.skip(b) {
				continue
			}
			pending = append(pending, b)
			evs, reply, rest := Parse(pending)
			pending = append([]byte(nil), rest...)
			if len(reply) > 0 {
				_, _ = p.Write(reply)
			}
			if len(rest) > 0 {
				restart(esc, 80*time.Millisecond)
			} else {
				stopTimer(esc)
			}
			if a.apply(evs) {
				a.quit = true
				return
			}
		case <-esc.C:
			evs := Flush(pending)
			pending = nil
			if a.apply(evs) {
				a.quit = true
				return
			}
		}
	}
}

func (a *App) apply(evs []Event) bool {
	for _, ev := range evs {
		if a.timeUp() {
			return true
		}
		if a.on(ev) {
			return true
		}
	}
	return false
}

func (a *App) on(ev Event) bool {
	a.note = ""
	switch a.mode {
	case modeRead:
		return a.onRead(ev)
	case modeReply:
		return a.onReply(ev)
	case modeProto:
		return a.onProto(ev)
	case modeAsk:
		return a.onAsk(ev)
	case modeUpAsk:
		return a.onUpAsk(ev)
	case modeSearch:
		return a.onSearch(ev)
	case modeHelp:
		return a.onHelp(ev)
	default:
		return a.onList(ev)
	}
}

func (a *App) onList(ev Event) bool {
	switch ev.Kind {
	case KindUp:
		a.move(a.sel - 1)
	case KindDown:
		a.move(a.sel + 1)
	case KindPgUp, KindLeft:
		a.page(-1)
	case KindPgDn, KindRight:
		a.page(1)
	case KindHome:
		a.move(0)
	case KindEnd:
		a.move(a.count() - 1)
	case KindEnter:
		if len(a.msgs) == 0 {
			return false
		}
		a.loadBody()
		a.mode = modeRead
		a.paintAll()
	case KindEsc:
		if a.allMsgs != nil {
			a.clearSearch()
			return false
		}
		return true
	case KindByte:
		switch ev.Ch {
		case 'q', 'Q':
			if a.allMsgs != nil {
				a.clearSearch()
				return false
			}
			return true
		case 'p', 'P':
			a.beginPost()
		case 'r', 'R':
			a.beginReply()
		case 'k', 'K':
			a.killMessage()
		case 's', 'S':
			a.beginSearch()
		case '?':
			a.beginHelp()
		}
	}
	return false
}

func (a *App) onRead(ev Event) bool {
	switch ev.Kind {
	case KindUp:
		a.scrollBody(-1)
	case KindDown:
		a.scrollBody(1)
	case KindPgUp:
		a.scrollBody(-bodyRows)
	case KindPgDn, KindEnter:
		a.scrollBody(bodyRows)
	case KindHome:
		a.bodyTop = 0
		a.paintBody()
		a.paintReadPos()
	case KindEnd:
		a.bodyTop = len(a.body) - bodyRows
		if a.bodyTop < 0 {
			a.bodyTop = 0
		}
		a.paintBody()
		a.paintReadPos()
	case KindLeft:
		a.stepRead(-1)
	case KindRight:
		a.stepRead(1)
	case KindEsc:
		a.backToList()
	case KindByte:
		switch ev.Ch {
		case 'q', 'Q':
			a.backToList()
		case 'n', 'N', '+':
			a.stepRead(1)
		case 'b', 'B', '-':
			a.stepRead(-1)
		case 'r', 'R':
			a.beginReply()
		case 'p', 'P':
			a.beginPost()
		case 'd', 'D':
			a.downloadMessage()
		case 'f', 'F':
			a.downloadFiles()
		case 'k', 'K':
			a.killMessage()
		case 's', 'S':
			a.beginSearch()
		case '?':
			a.beginHelp()
		case ' ':
			a.scrollBody(bodyRows)
		}
	}
	return false
}

func (a *App) backToList() {
	a.mode = modeList
	a.paintAll()
}

func (a *App) killMessage() {
	if a.sel < 0 || a.sel >= len(a.msgs) || a.base == nil {
		a.note = "No message to delete."
		a.paintAll()
		return
	}
	h := a.msgs[a.sel]
	if err := a.base.Delete(h); err != nil {
		a.note = err.Error()
		a.paintAll()
		return
	}
	a.note = "Deleted message " + strconv.FormatUint(uint64(h.Number), 10)
	a.msgs = append(a.msgs[:a.sel], a.msgs[a.sel+1:]...)
	a.forgetMessage(h.Number)
	if a.sel >= len(a.msgs) {
		a.sel = len(a.msgs) - 1
	}
	if a.sel < 0 {
		a.sel = 0
	}
	if a.top > a.sel {
		a.top = a.sel
	}
	if a.sel >= a.top+listRows {
		a.top = a.sel - listRows + 1
	}
	if len(a.msgs) == 0 {
		a.mode = modeList
		a.body = nil
		a.paintAll()
		return
	}
	if a.mode == modeRead {
		a.loadBody()
	}
	a.paintAll()
}

func (a *App) markSeen(h jam.Header) {
	if a.base == nil {
		return
	}
	// DOOR32 has the 1-based user record. EleBBS writes the 0-based one to .JLR.
	id := uint32(0)
	if a.user.UserNumber > 0 {
		id = uint32(a.user.UserNumber - 1)
	}
	toUser := eqName(h.To, a.user.RealName) || eqName(h.To, a.user.Alias)
	high, attr, err := a.base.Seen(h, id, toUser, a.user.RealName, a.user.Alias)
	if err != nil {
		if a.note == "" {
			a.note = err.Error()
		}
		return
	}
	if high > a.high {
		a.high = high
	}
	a.setAttr(h.Number, attr)
}

func (a *App) stepRead(delta int) {
	n := a.sel + delta
	if n < 0 || n >= len(a.msgs) {
		return
	}
	a.sel = n
	if a.sel < a.top {
		a.top = a.sel
	} else if a.sel >= a.top+listRows {
		a.top = a.sel - listRows + 1
	}
	a.loadBody()
	a.paintAll()
}

func (a *App) move(dest int) {
	oldSel, oldTop := a.sel, a.top
	a.sel, a.top = slide(a.sel, a.top, listRows, a.count(), dest)
	if a.sel == oldSel && a.top == oldTop {
		return
	}
	if a.top != oldTop {
		a.paintChoices()
	} else {
		a.barOff(oldSel)
		a.barOn(a.sel)
	}
	a.paintStatus()
}

// page flips the list a full screen, keeping the lightbar on the same row.
// At the first or last page it moves to the first or last message.
func (a *App) page(dir int) {
	n := a.count()
	if n == 0 {
		return
	}
	maxTop := n - listRows
	if maxTop < a.top {
		maxTop = a.top
	}
	if maxTop < 0 {
		maxTop = 0
	}
	top := a.top + dir*listRows
	if top < 0 {
		top = 0
	}
	if top > maxTop {
		top = maxTop
	}
	sel := a.sel + (top - a.top)
	if top == a.top {
		sel = 0
		if dir > 0 {
			sel = n - 1
		}
	}
	if sel < 0 {
		sel = 0
	}
	if sel >= n {
		sel = n - 1
	}
	if sel == a.sel && top == a.top {
		return
	}
	a.sel, a.top = sel, top
	a.paintChoices()
	a.paintStatus()
}

func (a *App) scrollBody(delta int) {
	maxTop := len(a.body) - bodyRows
	if maxTop < 0 {
		maxTop = 0
	}
	n := a.bodyTop + delta
	if n < 0 {
		n = 0
	}
	if n > maxTop {
		n = maxTop
	}
	if n == a.bodyTop {
		return
	}
	a.bodyTop = n
	a.paintBody()
	a.paintReadPos()
}

func (a *App) count() int {
	return len(a.msgs)
}

func (a *App) timeUp() bool {
	mins, unlimited := a.user.Remaining(time.Now())
	if unlimited {
		return false
	}
	return mins <= 0 && time.Since(a.user.Started) >= time.Minute
}

func (a *App) openArea(i int) {
	if a.base != nil {
		a.base.Close()
		a.base = nil
	}
	a.msgs = nil
	a.dropSearch()
	a.err = ""
	a.area = i
	if i < 0 || i >= len(a.areas) {
		a.err = "No area selected."
		a.sel, a.top = 0, 0
		return
	}
	b, err := jam.Open(a.areas[i].Path)
	if err != nil {
		a.err = err.Error()
		a.sel, a.top = 0, 0
		return
	}
	a.base = b
	a.high = b.HighRead(a.user.RealName, a.user.Alias)
	msgs, err := b.List()
	if err != nil {
		a.err = err.Error()
		a.sel, a.top = 0, 0
		return
	}
	msgs = a.visible(msgs)
	a.msgs = msgs
	a.sel = len(msgs) - 1
	if a.sel < 0 {
		a.sel = 0
	}
	unread := false
	for n, m := range msgs {
		if a.unread(m) {
			a.sel = n
			unread = true
			break
		}
	}
	a.top = 0
	if unread {
		a.top = a.sel
		if last := len(msgs) - listRows; a.top > last {
			a.top = last
		}
		if a.top < 0 {
			a.top = 0
		}
	} else if a.sel >= listRows {
		a.top = a.sel - listRows + 1
	}
}

func (a *App) forUser(h jam.Header) bool {
	return eqName(h.From, a.user.RealName) || eqName(h.From, a.user.Alias) ||
		eqName(h.To, a.user.RealName) || eqName(h.To, a.user.Alias)
}

// canSee is EleBBS ReadMsgAccess: private mail is shown to its sender,
// its recipient, and users with sysop access to the area. Mail to All
// is not treated as private.
func (a *App) canSee(h jam.Header) bool {
	if !h.Private() || isAll(h.To) || a.forUser(h) {
		return true
	}
	return a.area >= 0 && a.area < len(a.areas) && a.areas[a.area].SysopAccess
}

func (a *App) visible(msgs []jam.Header) []jam.Header {
	out := msgs[:0]
	for _, m := range msgs {
		if a.canSee(m) {
			out = append(out, m)
		}
	}
	return out
}

// mark is * for unread. + is someone else's private mail, which only a
// sysop sees.
func (a *App) mark(h jam.Header) string {
	if a.unread(h) {
		return "*"
	}
	if h.Private() && !isAll(h.To) && !a.forUser(h) {
		return "+"
	}
	return " "
}

// unread is the bright "new" mark. Private mail is new only to its
// recipient, until the received bit is set. Public messages use the
// lastread high-water mark.
func (a *App) unread(h jam.Header) bool {
	toMe := eqName(h.To, a.user.RealName) || eqName(h.To, a.user.Alias)
	if h.Private() && !isAll(h.To) {
		return toMe && !h.Read()
	}
	if toMe && h.Read() {
		return false
	}
	return h.Number > a.high
}

func (a *App) subject(h jam.Header) string {
	if !a.canSee(h) {
		return "Private"
	}
	return h.Subject
}

func (a *App) listSubject(h jam.Header) string {
	if !a.canSee(h) {
		return "Private"
	}
	if names := a.attachNames(h); len(names) > 0 {
		return strings.Join(names, ", ")
	}
	return h.Subject
}

func (a *App) loadBody() {
	a.body = nil
	a.bodyTop = 0
	if a.sel < 0 || a.sel >= len(a.msgs) || a.base == nil {
		return
	}
	h := a.msgs[a.sel]
	if !a.canSee(h) {
		a.body = []string{"This message is private."}
		a.markSeen(h)
		return
	}
	raw, err := a.base.Text(h)
	if err != nil {
		a.body = []string{err.Error()}
		a.markSeen(h)
		return
	}
	a.body = bodyLines(raw, ansiWidth)
	a.markSeen(h)
}

func (a *App) paintAll() {
	a.scr.begin()
	switch a.mode {
	case modeRead:
		a.paintRead()
		return
	case modeReply:
		a.paintReply()
		return
	case modeProto:
		a.paintProto()
		return
	case modeAsk:
		a.paintAsk()
		return
	case modeUpAsk:
		a.paintUpAsk()
		return
	case modeSearch:
		a.paintSearch()
		return
	case modeHelp:
		a.paintHelp()
		return
	}
	title := "Elereader"
	if a.area >= 0 && a.area < len(a.areas) {
		title = "Elereader  " + a.areas[a.area].Name
	}
	help := "Up/Dn  Enter Read  P Post  R Reply  K Kill  S Search  ? Help  Q Quit"
	if a.allMsgs != nil {
		title += "  Search: " + a.searchTitle
		help = "Up/Dn  Enter Read  P Post  R Reply  K Kill  S Search  ? Help  Q Back"
	}
	a.scr.rule(1, chTL, chTR, title)
	a.scr.content(2, attrHead, columnHead())
	a.paintChoices()
	a.scr.rule(22, chJL, chJR, a.note)
	a.paintStatus()
	a.scr.rule(24, chBL, chBR, help)
}

func (a *App) paintRead() {
	h := jam.Header{}
	if a.sel >= 0 && a.sel < len(a.msgs) {
		h = a.msgs[a.sel]
	}
	a.scr.rule(1, chTL, chTR, fmt.Sprintf("Message %d", h.Number))
	when := ""
	if !h.When.IsZero() {
		when = h.When.Format("02 Jan 06 15:04")
	}
	a.scr.content(2, attrNorm, fieldLine("From", h.From, when))
	a.scr.content(3, attrNorm, fieldLine("To", h.To, a.kindLabel(h)))
	a.scr.content(4, attrNorm, fieldLine("Subj", a.subject(h), ""))
	names := a.attachNames(h)
	if len(names) == 0 {
		a.scr.rule(5, chJL, chJR, "")
	} else {
		a.scr.content(5, attrHead, fit(" File "+strings.Join(names, ", "), contentWidth))
	}
	a.paintBody()
	a.paintReadPos()
	if a.note != "" {
		a.scr.content(22, attrNorm, fit(" "+a.note, contentWidth))
	}
	a.scr.content(statusY, attrNorm, a.statusText())
	a.scr.rule(24, chBL, chBR, "Lt/Rt  R Reply  P Post  K Kill  S Search  D/F Download  ? Help  Q Back")
}

func (a *App) paintChoices() {
	var errLines []string
	if a.err != "" && a.count() == 0 {
		errLines = wrapVisible([]byte(a.err), contentWidth)
	}
	for row := 0; row < listRows; row++ {
		i := a.top + row
		if i < 0 || i >= a.count() {
			text := blank(contentWidth)
			if row < len(errLines) {
				text = padVisible(errLines[row], contentWidth)
			}
			a.scr.content(listY+row, attrNorm, text)
			continue
		}
		a.paintIndex(i, i == a.sel)
	}
}

func (a *App) barOn(i int)  { a.paintIndex(i, true) }
func (a *App) barOff(i int) { a.paintIndex(i, false) }

func (a *App) paintIndex(i int, selected bool) {
	if i < a.top || i >= a.top+listRows {
		return
	}
	y := listY + (i - a.top)
	color := attrNorm
	if selected {
		color = attrBar
	}
	if !selected && a.unread(a.msgs[i]) {
		color = attrNew
	}
	h := a.msgs[i]
	a.scr.content(y, color, msgLine(h, a.mark(h), a.listSubject(h)))
}

func (a *App) paintBody() {
	for i := 0; i < bodyRows; i++ {
		text := blank(ansiWidth)
		n := a.bodyTop + i
		if n >= 0 && n < len(a.body) {
			text = []byte(a.body[n])
		}
		a.scr.row(bodyY+i, text)
	}
}

func (a *App) paintReadPos() {
	total := len(a.body)
	msg := " (no text)"
	if total > 0 {
		from := a.bodyTop + 1
		to := a.bodyTop + bodyRows
		if to > total {
			to = total
		}
		msg = fmt.Sprintf(" Lines %d-%d of %d", from, to, total)
	}
	a.scr.content(22, attrNorm, fit(msg, contentWidth))
}

func (a *App) paintStatus() {
	a.scr.content(statusY, attrNorm, a.statusText())
}

func (a *App) statusText() []byte {
	mins, unlimited := a.user.Remaining(time.Now())
	left := "no limit"
	if !unlimited {
		left = fmt.Sprintf("%d min", mins)
	}
	total := a.count()
	pos := 0
	if total > 0 {
		pos = a.sel + 1
	}
	where := "Messages"
	if a.area >= 0 && a.area < len(a.areas) && a.areas[a.area].Name != "" {
		where = a.areas[a.area].Name
	}
	s := fmt.Sprintf(" %s   %s   %d/%d   node %d   %s", a.user.Who(), where, pos, total, a.user.Node, left)
	return fit(s, contentWidth)
}

func columnHead() []byte {
	b := blank(contentWidth)
	copy(b[1:], "#")
	copy(b[7:], "From")
	copy(b[24:], "To")
	copy(b[37:], "Subject")
	copy(b[65:], "Date")
	return b
}

func msgLine(h jam.Header, mark, sub string) []byte {
	b := blank(contentWidth)
	if mark != "" {
		b[0] = mark[0]
	}
	num := strconv.FormatUint(uint64(h.Number), 10)
	if len(num) > 5 {
		num = num[len(num)-5:]
	}
	copy(b[1:], fmt.Sprintf("%5s", num))
	copy(b[7:], fit(h.From, 16))
	copy(b[24:], fit(h.To, 12))
	copy(b[37:], fit(sub, 27))
	copy(b[65:], fit(h.When.Format("02-Jan-06"), 9))
	return b
}

func fieldLine(label, value, extra string) []byte {
	b := blank(contentWidth)
	copy(b[1:], fit(label, 6))
	copy(b[8:], fit(value, 36))
	if extra != "" {
		copy(b[48:], fit(extra, 28))
	}
	return b
}

func restart(t *time.Timer, d time.Duration) {
	stopTimer(t)
	t.Reset(d)
}

func stopTimer(t *time.Timer) {
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
}
