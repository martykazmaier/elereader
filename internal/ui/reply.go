package ui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"elereader/internal/jam"
)

func (a *App) beginPost() {
	if a.base == nil {
		a.note = "Message base is not open."
		a.paintAll()
		return
	}
	a.replyFrom = a.mode
	a.postNew = true
	a.replyTo = defaultNewTo(a.kind())
	a.replySub = ""
	a.replyAddr = ""
	a.replyFld = 0
	a.replyEdit = false
	a.attaches = nil
	a.quote = nil
	a.replyNote = postHint(a.kind())
	a.openComposer()
}

func (a *App) beginReply() {
	if a.sel < 0 || a.sel >= len(a.msgs) {
		a.note = "No message to reply to."
		a.paintAll()
		return
	}
	if a.mode != modeRead {
		a.loadBody()
	}
	h := a.msgs[a.sel]
	a.replyFrom = a.mode
	a.postNew = false
	a.replyTo = defaultTo(a.kind(), h.From)
	a.replySub = replySubject(h.Subject)
	a.replyAddr = ""
	if a.kind() == jam.AreaNetmail {
		a.replyAddr = h.Origin
	}
	a.replyFld = 0
	a.replyEdit = false
	a.attaches = nil
	a.replyNote = replyHint(a.kind())
	a.quote = quoteLines(a.body)
	a.openComposer()
}

func (a *App) openComposer() {
	a.replyFld = 0
	a.replyEdit = true
	a.replyBuf = a.fieldValue(0)
	a.replyHold = a.replyBuf
	a.mode = modeReply
	a.paintAll()
}

func (a *App) onReply(ev Event) bool {
	if a.replyEdit {
		return a.onEdit(ev)
	}
	switch ev.Kind {
	case KindUp:
		a.moveReplyField(-1)
	case KindDown:
		a.moveReplyField(1)
	case KindEnter:
		if a.replyFld == a.lastReplyField() {
			a.askUpload()
		} else {
			a.replyEdit = true
			a.replyHold = a.fieldValue(a.replyFld)
			a.replyBuf = a.replyHold
			a.paintReply()
		}
	case KindEsc:
		a.leaveComposer()
	case KindByte:
		switch ev.Ch {
		case 'q', 'Q':
			return true
		case 'a', 'A':
			a.replyFld = 2
			a.replyEdit = true
			a.replyHold = ""
			a.replyBuf = ""
			a.replyNote = "Path of the file to attach. Enter adds it."
			a.paintReply()
		}
	}
	return false
}

func (a *App) onEdit(ev Event) bool {
	switch ev.Kind {
	case KindEnter:
		a.storeField(a.replyFld, strings.TrimSpace(a.replyBuf))
		if a.replyFld == 2 {
			a.replyEdit = false
			a.replyFld = a.lastReplyField()
			a.paintReply()
			return false
		}
		if a.replyFld == a.lastReplyField() {
			a.askUpload()
			return false
		}
		a.advanceReplyField()
	case KindUp:
		a.storeField(a.replyFld, strings.TrimSpace(a.replyBuf))
		a.moveReplyField(-1)
		a.beginFieldEdit()
	case KindDown:
		a.storeField(a.replyFld, strings.TrimSpace(a.replyBuf))
		a.moveReplyField(1)
		a.beginFieldEdit()
	case KindEsc:
		a.leaveComposer()
	case KindByte:
		if ev.Ch == 8 || ev.Ch == 127 {
			if len(a.replyBuf) > 0 {
				a.replyBuf = a.replyBuf[:len(a.replyBuf)-1]
				a.paintReply()
			}
			return false
		}
		if ev.Ch >= 32 && len(a.replyBuf) < 70 {
			a.replyBuf += string(ev.Ch)
			a.paintReply()
		}
	}
	return false
}

func (a *App) askUpload() {
	a.replyEdit = false
	if msg := validateReply(a.kind(), a.replyTo, a.replySub, a.replyAddr); msg != "" {
		a.focusInvalid(msg)
		return
	}
	a.mode = modeUpAsk
	a.paintAll()
}

func (a *App) paintUpAsk() {
	title := "Reply"
	if a.postNew {
		title = "Post"
	}
	a.scr.rule(1, chTL, chTR, title)
	a.scr.content(2, attrNorm, blank(contentWidth))
	a.scr.content(3, attrNorm, fit(" Upload a message?", contentWidth))
	for y := 4; y <= 22; y++ {
		a.scr.content(y, attrNorm, blank(contentWidth))
	}
	a.scr.content(statusY, attrNorm, a.statusText())
	a.scr.rule(24, chBL, chBR, "Y Yes  N No  Esc Back")
}

func (a *App) onUpAsk(ev Event) bool {
	switch ev.Kind {
	case KindEsc:
		a.mode = modeReply
		a.paintAll()
	case KindEnter:
		a.writeMessage()
	case KindByte:
		switch ev.Ch {
		case 'y', 'Y':
			a.beginMsgUpload()
		case 'n', 'N':
			a.writeMessage()
		case 'q', 'Q':
			return true
		}
	}
	return false
}

func (a *App) writeMessage() {
	a.replyEdit = false
	if msg := validateReply(a.kind(), a.replyTo, a.replySub, a.replyAddr); msg != "" {
		a.focusInvalid(msg)
		return
	}
	dir := a.workDir()
	bodyPath, err := a.prepareEditor(dir)
	if err != nil {
		a.replyNote = err.Error()
		a.paintReply()
		return
	}
	env := a.xferEnv()
	line, err := editorArgs(a.editorCommand(), env.port, env.baud, env.mins, "60")
	if err != nil {
		a.replyNote = err.Error()
		a.paintReply()
		return
	}
	back := a.lendCaller()
	defer back()
	err = runInherited(line, dir, a.user.Handle)
	_ = os.Remove(filepath.Join(dir, "msginf"))
	var exited programExit
	if err != nil && !errors.As(err, &exited) {
		a.note = err.Error()
		a.leaveComposer()
		return
	}
	body, readErr := os.ReadFile(bodyPath)
	if readErr != nil || len(strings.TrimSpace(string(body))) == 0 {
		a.note = "Message was not saved."
		a.leaveComposer()
		return
	}
	a.editPath = bodyPath
	a.mode = modeAsk
	a.paintAll()
}

func (a *App) storeField(n int, val string) {
	switch n {
	case 0:
		a.replyTo = val
	case 1:
		a.replySub = val
	case 3:
		a.replyAddr = val
	case 2:
		if val == "" {
			return
		}
		if _, err := os.Stat(val); err != nil {
			a.replyNote = "File not found."
			return
		}
		a.attaches = append(a.attaches, val)
		a.replyNote = "Attached " + filepath.Base(val)
	}
}

func (a *App) lastReplyField() int {
	if a.kind() == jam.AreaNetmail {
		return 3
	}
	return 1
}

func (a *App) advanceReplyField() {
	fields := []int{0, 1}
	if a.kind() == jam.AreaNetmail {
		fields = append(fields, 3)
	}
	for i, id := range fields {
		if id == a.replyFld && i+1 < len(fields) {
			a.replyFld = fields[i+1]
			break
		}
	}
	a.beginFieldEdit()
}

func (a *App) beginFieldEdit() {
	a.replyEdit = true
	a.replyBuf = a.fieldValue(a.replyFld)
	a.replyHold = a.replyBuf
	a.paintReply()
}

func (a *App) focusInvalid(msg string) {
	a.replyNote = msg
	switch {
	case strings.Contains(msg, "Subject"):
		a.replyFld = 1
	case strings.Contains(msg, "1:2/3"):
		a.replyFld = 3
	default:
		a.replyFld = 0
	}
	a.beginFieldEdit()
}

func (a *App) sendReply() {
	if msg := validateReply(a.kind(), a.replyTo, a.replySub, a.replyAddr); msg != "" {
		a.replyNote = msg
		a.paintReply()
		return
	}
	body, err := os.ReadFile(a.editPath)
	if err != nil || len(strings.TrimSpace(string(body))) == 0 {
		a.replyNote = "The message is empty."
		a.paintReply()
		return
	}
	if a.base == nil {
		a.replyNote = "Message base is not open."
		a.paintReply()
		return
	}
	var names []string
	subject := strings.TrimSpace(a.replySub)
	fileAttach := false
	if dir := a.uploadDir; dir != "" && len(dirFiles(dir)) > 0 {
		subject = forceBack(dir)
		fileAttach = true
	} else {
		for _, src := range a.attaches {
			name, err := stageAttach(a.attachDir(), src)
			if err != nil {
				a.replyNote = err.Error()
				a.paintReply()
				return
			}
			names = append(names, name)
		}
	}
	var orig jam.Header
	replyNum := uint32(0)
	toAddr := ""
	if !a.postNew && a.sel >= 0 && a.sel < len(a.msgs) {
		orig = a.msgs[a.sel]
		replyNum = orig.Number
		toAddr = orig.Origin
	}
	kind := a.kind()
	if kind == jam.AreaNetmail {
		toAddr = strings.TrimSpace(a.replyAddr)
	}
	num, err := a.base.Post(jam.Outgoing{
		From:     a.fromName(),
		To:       strings.TrimSpace(a.replyTo),
		Subject:  subject,
		Text:     normalizeNewlines(body),
		Kind:     kind,
		Private:  replyPrivate(kind, a.replyTo),
		ReplyTo:  replyNum,
		FromAddr: a.current().Origin,
		ToAddr:   toAddr,
		Files:    names,
		Attach:   fileAttach,
	})
	if err != nil {
		a.replyNote = err.Error()
		a.paintReply()
		return
	}
	a.note = "Posted message " + itoa(num)
	a.uploadDir = ""
	a.protoUp = false
	a.refresh(num)
	a.mode = modeRead
	a.loadBody()
	a.paintAll()
}

func (a *App) paintReply() {
	kind := a.kind().Label()
	title := "Reply  " + kind
	if a.postNew {
		title = "Post  " + kind
	}
	a.scr.rule(1, chTL, chTR, title)
	a.scr.content(2, attrNorm, fieldLine("From", a.fromName(), kind))
	a.scr.content(3, a.fieldColor(0), fieldLine("To", a.fieldText(0), ""))
	a.scr.content(4, a.fieldColor(1), fieldLine("Subj", a.fieldText(1), ""))
	next := 5
	if a.kind() == jam.AreaNetmail {
		a.scr.content(5, a.fieldColor(3), fieldLine("Addr", a.fieldText(3), ""))
		next = 6
	}
	for y := next; y <= 19; y++ {
		a.scr.content(y, attrNorm, blank(contentWidth))
	}
	fileLine := "No file attached"
	if a.replyEdit && a.replyFld == 2 {
		fileLine = a.replyBuf + "_"
	} else if len(a.attaches) > 0 {
		var names []string
		for _, p := range a.attaches {
			names = append(names, filepath.Base(p))
		}
		fileLine = "Files: " + strings.Join(names, ", ")
	}
	a.scr.content(20, a.fieldColor(2), fit(" "+fileLine, contentWidth))
	a.scr.content(21, attrNorm, fit(" "+a.replyNote, contentWidth))
	a.scr.content(22, attrNorm, blank(contentWidth))
	a.scr.content(statusY, attrNorm, a.statusText())
	a.scr.rule(24, chBL, chBR, "Enter  Esc Cancel")
}

func (a *App) paintAsk() {
	title := "Reply"
	if a.postNew {
		title = "Post"
	}
	a.scr.rule(1, chTL, chTR, title)
	a.scr.content(3, attrNorm, fit(" Attach a file to this message?", contentWidth))
	for y := 4; y <= 22; y++ {
		a.scr.content(y, attrNorm, blank(contentWidth))
	}
	a.scr.content(statusY, attrNorm, a.statusText())
	a.scr.rule(24, chBL, chBR, "Y Yes  N No")
}

func (a *App) onAsk(ev Event) bool {
	switch ev.Kind {
	case KindEsc:
		a.sendReply()
	case KindByte:
		switch ev.Ch {
		case 'y', 'Y':
			if !a.current().AllowAttach {
				a.sendReply()
				a.note = "This area does not allow file attaches."
				if a.mode == modeRead {
					a.paintAll()
				}
				return false
			}
			a.beginUpload()
		case 'n', 'N':
			a.sendReply()
		case 'q', 'Q':
			return true
		}
	}
	return false
}

func (a *App) fieldColor(n int) string {
	if a.replyFld == n && (n != 2 || a.replyEdit) {
		return attrBar
	}
	return attrNorm
}

func (a *App) fieldText(n int) string {
	if a.replyEdit && a.replyFld == n {
		s := a.replyBuf
		if len(s) < 60 {
			s += "_"
		}
		return s
	}
	if n == 0 {
		return a.replyTo
	}
	if n == 3 {
		return a.replyAddr
	}
	return a.replySub
}

func (a *App) prepareEditor(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	err := writeMsgInf(filepath.Join(dir, "msginf"), a.fromName(), strings.TrimSpace(a.replyTo), strings.TrimSpace(a.replySub), a.nextMsgNum(), a.current().Name, replyPrivate(a.kind(), a.replyTo))
	if err != nil {
		return "", err
	}
	bodyPath := filepath.Join(dir, "msgtmp")
	if a.postNew || len(a.quote) == 0 {
		_ = os.Remove(bodyPath)
		return bodyPath, nil
	}
	var b strings.Builder
	for _, ln := range a.quote {
		b.WriteString(ln)
		b.WriteString("\r\n")
	}
	return bodyPath, os.WriteFile(bodyPath, []byte(b.String()), 0644)
}

func (a *App) nextMsgNum() string {
	var high uint32
	for _, m := range a.msgs {
		if m.Number > high {
			high = m.Number
		}
	}
	return itoa(high + 1)
}

func writeMsgInf(path, from, to, subject, number, area string, private bool) error {
	flag := "NO"
	if private {
		flag = "YES"
	}
	text := from + "\r\n" + to + "\r\n" + subject + "\r\n" + number + "\r\n" + area + "\r\n" + flag + "\r\n"
	return os.WriteFile(path, []byte(text), 0644)
}

func (a *App) refresh(number uint32) {
	if a.base == nil {
		return
	}
	a.high = a.base.HighRead(a.user.RealName, a.user.Alias)
	msgs, err := a.base.List()
	if err != nil {
		a.note = err.Error()
		return
	}
	msgs = a.visible(msgs)
	a.msgs = msgs
	a.sel = len(msgs) - 1
	for i, m := range msgs {
		if m.Number == number {
			a.sel = i
			break
		}
	}
	if a.sel < 0 {
		a.sel = 0
	}
	a.top = 0
	if a.sel >= listRows {
		a.top = a.sel - listRows + 1
	}
	a.loadBody()
	a.mode = modeRead
}

func (a *App) kind() jam.AreaKind {
	return a.current().Kind
}

func (a *App) kindLabel(h jam.Header) string {
	label := a.kind().Label()
	if h.Private() {
		label = "Private " + label
	}
	if len(h.Files) > 0 {
		label += " +file"
	}
	return label
}

func (a *App) current() Area {
	if a.area >= 0 && a.area < len(a.areas) {
		return a.areas[a.area]
	}
	return Area{}
}

func (a *App) fromName() string {
	if a.kind() == jam.AreaEcho && a.user.Alias != "" {
		return a.user.Alias
	}
	if a.user.RealName != "" {
		return a.user.RealName
	}
	return a.user.Who()
}

func (a *App) workDir() string {
	if dir := a.current().Node; dir != "" {
		return dir
	}
	return os.TempDir()
}

func (a *App) attachDir() string {
	if dir := a.current().Attach; dir != "" {
		return dir
	}
	return filepath.Join(a.workDir(), "attach")
}

func (a *App) editorCommand() string {
	if cmd := strings.TrimSpace(a.current().Editor); cmd != "" {
		return cmd
	}
	if cmd := strings.TrimSpace(os.Getenv("EDITOR")); cmd != "" {
		return cmd
	}
	return ""
}

func (a *App) moveReplyField(delta int) {
	fields := []int{0, 1}
	if a.kind() == jam.AreaNetmail {
		fields = append(fields, 3)
	}
	i := 0
	for n, id := range fields {
		if id == a.replyFld {
			i = n
			break
		}
	}
	i += delta
	if i < 0 {
		i = len(fields) - 1
	}
	if i >= len(fields) {
		i = 0
	}
	a.replyFld = fields[i]
	a.paintReply()
}

func (a *App) leaveComposer() {
	a.mode = a.replyFrom
	if a.mode != modeList && a.mode != modeRead {
		a.mode = modeList
	}
	a.paintAll()
}

func (a *App) fieldValue(n int) string {
	switch n {
	case 0:
		return a.replyTo
	case 1:
		return a.replySub
	case 3:
		return a.replyAddr
	default:
		return ""
	}
}

func (a *App) restoreField(n int, val string) {
	switch n {
	case 0:
		a.replyTo = val
	case 1:
		a.replySub = val
	case 3:
		a.replyAddr = val
	}
}

func defaultNewTo(kind jam.AreaKind) string {
	switch kind {
	case jam.AreaEcho, jam.AreaLocal:
		return "All"
	default:
		return ""
	}
}

func defaultTo(kind jam.AreaKind, from string) string {
	from = strings.TrimSpace(from)
	if from == "" && kind == jam.AreaEcho {
		return "All"
	}
	return from
}

func replySubject(subject string) string {
	s := strings.TrimSpace(subject)
	if s == "" {
		return "Re:"
	}
	if len(s) >= 3 && strings.EqualFold(s[:3], "re:") {
		return s
	}
	return "Re: " + s
}

func postHint(kind jam.AreaKind) string {
	switch kind {
	case jam.AreaNetmail:
		return "Enter To, Subject, and the netmail address. The editor opens and the message is saved."
	case jam.AreaEmail:
		return "To is an internet address. The editor opens and the message is saved."
	default:
		return "Enter To and Subject. The editor opens and the message is saved."
	}
}

func replyHint(kind jam.AreaKind) string {
	switch kind {
	case jam.AreaEmail:
		return "To is an internet address. The editor opens and the message is saved."
	case jam.AreaNetmail:
		return "Change To, Subject, or the netmail address. The editor opens and the message is saved."
	case jam.AreaEcho:
		return "To can be All or a name. The editor opens and the message is saved."
	default:
		return "To can be All or a user. The editor opens and the message is saved."
	}
}

func validateReply(kind jam.AreaKind, to, subject, addr string) string {
	to = strings.TrimSpace(to)
	subject = strings.TrimSpace(subject)
	addr = strings.TrimSpace(addr)
	if subject == "" {
		return "Subject is required."
	}
	switch kind {
	case jam.AreaEmail:
		if !strings.Contains(to, "@") || strings.HasPrefix(to, "@") || strings.HasSuffix(to, "@") {
			return "Email needs an address in To."
		}
	case jam.AreaNetmail:
		if to == "" || isAll(to) {
			return "Netmail needs a recipient in To."
		}
		if !netmailAddr(addr) {
			return "Netmail needs an address, like 1:2/3."
		}
	default:
		if to == "" {
			return "To is required. Use All for a public post."
		}
	}
	return ""
}

func netmailAddr(s string) bool {
	i := strings.IndexByte(s, ':')
	j := strings.IndexByte(s, '/')
	return i > 0 && j > i+1 && j < len(s)-1
}

func replyPrivate(kind jam.AreaKind, to string) bool {
	switch kind {
	case jam.AreaNetmail, jam.AreaEmail:
		return true
	case jam.AreaLocal:
		return !isAll(to)
	default:
		return false
	}
}

func isAll(to string) bool {
	s := strings.ToLower(strings.TrimSpace(to))
	return s == "all" || s == "all users"
}

func quoteLines(lines []string) []string {
	var out []string
	for _, ln := range lines {
		plain := strings.TrimRight(string(stripSGR([]byte(ln))), " ")
		if strings.TrimSpace(plain) == "" {
			continue
		}
		out = append(out, "> "+plain)
		if len(out) == 30 {
			break
		}
	}
	return out
}

func normalizeNewlines(b []byte) []byte {
	s := strings.ReplaceAll(string(b), "\r\n", "\n")
	s = strings.ReplaceAll(s, "\n", "\r")
	return []byte(s)
}

func editorArgs(command, port, baud, mins, timeout string) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", fmt.Errorf("ExternalEdCmd is not set")
	}
	return command + " " + port + " " + baud + " " + mins + " " + timeout, nil
}

// programExit is a finished external program. The editor's save is msgtmp,
// so a non-zero exit code does not by itself discard that file.
type programExit struct {
	Code uint32
}

func (e programExit) Error() string {
	return fmt.Sprintf("exit %d", e.Code)
}

func quoteArg(s string) string {
	if strings.ContainsAny(s, " \t") {
		return `"` + s + `"`
	}
	return s
}

func itoa(n uint32) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
