package ui

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (a *App) downloadMessage() {
	if a.sel < 0 || a.sel >= len(a.msgs) {
		return
	}
	h := a.msgs[a.sel]
	var b strings.Builder
	b.WriteString("From: " + h.From + "\r\n")
	b.WriteString("To: " + h.To + "\r\n")
	b.WriteString("Subject: " + h.Subject + "\r\n")
	if !h.When.IsZero() {
		b.WriteString("Date: " + h.When.Format("02 Jan 2006 15:04") + "\r\n")
	}
	b.WriteString("Area: " + a.kind().Label() + "\r\n")
	if names := a.attachNames(h); len(names) > 0 {
		b.WriteString("Files: " + strings.Join(names, ", ") + "\r\n")
	}
	b.WriteString("\r\n")
	for _, ln := range a.body {
		b.WriteString(strings.TrimRight(string(stripSGR([]byte(ln))), " "))
		b.WriteString("\r\n")
	}
	name := "msg-" + itoa(h.Number) + ".txt"
	path, err := a.export(name, []byte(b.String()))
	if err != nil {
		a.note = err.Error()
		a.paintAll()
		return
	}
	a.beginTransfer([]string{path})
}

func (a *App) downloadFiles() {
	if a.sel < 0 || a.sel >= len(a.msgs) {
		return
	}
	files := a.attachPaths(a.msgs[a.sel])
	if len(files) == 0 {
		a.note = "No file attached."
		a.paintAll()
		return
	}
	a.beginTransfer(files)
}

func (a *App) beginTransfer(files []string) {
	prots, err := a.downloadProtocols()
	if err != nil {
		a.note = err.Error()
		a.paintAll()
		return
	}
	a.protos = prots
	a.protoSel = 0
	a.protoTop = 0
	a.protoFiles = files
	a.protoUp = false
	a.mode = modeProto
	a.paintAll()
}

func (a *App) beginUpload() {
	dir, err := a.makeAttachDir()
	if err != nil {
		a.replyNote = err.Error()
		a.sendReply()
		return
	}
	a.uploadDir = dir
	prots, err := a.uploadProtocols()
	if err != nil {
		a.abandonUpload()
		a.sendReply()
		a.note = err.Error()
		if a.mode == modeRead {
			a.paintAll()
		}
		return
	}
	a.protos = prots
	a.protoSel = 0
	a.protoTop = 0
	a.protoFiles = nil
	a.protoUp = true
	a.mode = modeProto
	a.paintAll()
}

func (a *App) beginMsgUpload() {
	dir, err := a.makeAttachDir()
	if err != nil {
		a.msgUploadFailed(err.Error())
		return
	}
	prots, err := a.uploadProtocols()
	if err != nil {
		_ = os.RemoveAll(dir)
		a.msgUploadFailed(err.Error())
		return
	}
	a.uploadDir = dir
	a.protos = prots
	a.protoSel = 0
	a.protoTop = 0
	a.protoFiles = nil
	a.protoUp = true
	a.protoMsg = true
	a.mode = modeProto
	a.paintAll()
}

func (a *App) runMsgUpload(p Protocol, env xferEnv) {
	dir := a.uploadDir
	if err := writeUpControl(p, dir, env); err != nil {
		a.endMsgUpload()
		a.msgUploadFailed(err.Error())
		return
	}
	line := expandUpload(p.UpCmd, dir, env)
	a.eraseProtocolFiles(p, env)
	back := a.lendCaller(true)
	defer back()
	err := runExternal(line, a.workDir(), a.user.Handle, false)
	a.pullLogged(p, env)
	a.eraseProtocolFiles(p, env)
	setBlocking(a.port, true)
	body := uploadedText(dirFiles(dir))
	a.endMsgUpload()
	if len(strings.TrimSpace(string(body))) == 0 {
		if err != nil {
			a.msgUploadFailed(p.Name + " stopped: " + err.Error())
		} else {
			a.msgUploadFailed("No message received.")
		}
		return
	}
	path, _ := filepath.Abs(filepath.Join(a.workDir(), "msgtmp"))
	if err := os.WriteFile(path, body, 0644); err != nil {
		a.msgUploadFailed(err.Error())
		return
	}
	a.editPath = path
	a.mode = modeAsk
	a.paintAll()
}

func (a *App) endMsgUpload() {
	if a.uploadDir != "" {
		_ = os.RemoveAll(a.uploadDir)
	}
	a.uploadDir = ""
	a.protoUp = false
	a.protoMsg = false
}

func (a *App) msgUploadFailed(msg string) {
	a.replyNote = msg
	a.mode = modeReply
	a.paintAll()
}

// uploadedText is the largest received file. XMODEM pads the last block with
// Ctrl-Z, which is not message text.
func uploadedText(files []string) []byte {
	var best []byte
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err == nil && len(b) > len(best) {
			best = b
		}
	}
	if i := bytes.IndexByte(best, 0x1A); i >= 0 {
		best = best[:i]
	}
	return best
}

func (a *App) downloadProtocols() ([]Protocol, error) {
	return a.protocolsWith(false)
}

func (a *App) uploadProtocols() ([]Protocol, error) {
	return a.protocolsWith(true)
}

func (a *App) protocolsWith(upload bool) ([]Protocol, error) {
	all, err := loadProtocols(a.protocolPath())
	if err != nil {
		return nil, err
	}
	var out []Protocol
	for _, p := range all {
		cmd := p.DnCmd
		if upload {
			cmd = p.UpCmd
		}
		if strings.TrimSpace(cmd) != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		if upload {
			return nil, fmt.Errorf("PROTOCOL.RA has no external upload commands")
		}
		return nil, fmt.Errorf("PROTOCOL.RA has no external download commands")
	}
	return out, nil
}

func (a *App) makeAttachDir() (string, error) {
	root := a.attachDir()
	if err := os.MkdirAll(root, 0755); err != nil {
		return "", err
	}
	now := time.Now()
	stamp := now.Format("150405") + now.Format("020106")
	if len(stamp) > 8 {
		stamp = stamp[:8]
	}
	name := stamp + "." + itoa(uint32(a.user.Node))
	dir := filepath.Join(root, name)
	if err := os.Mkdir(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

func (a *App) abandonUpload() {
	if a.uploadDir == "" {
		return
	}
	if len(filesIn(a.uploadDir, a.attachDir())) == 0 {
		_ = os.Remove(a.uploadDir)
	}
	a.uploadDir = ""
	a.protoUp = false
}

func (a *App) onProto(ev Event) bool {
	switch ev.Kind {
	case KindUp:
		a.moveProto(-1)
	case KindDown:
		a.moveProto(1)
	case KindEnter:
		a.runProto(a.protoSel)
	case KindEsc:
		if a.protoMsg {
			a.endMsgUpload()
			a.mode = modeUpAsk
			a.paintAll()
			return false
		}
		if a.protoUp {
			a.abandonUpload()
			a.sendReply()
			return false
		}
		a.mode = modeRead
		a.paintAll()
	case KindByte:
		if ev.Ch == 'q' || ev.Ch == 'Q' {
			return true
		}
		for i, p := range a.protos {
			if p.Key == ev.Ch || p.Key == ev.Ch-32 || p.Key == ev.Ch+32 {
				a.runProto(i)
				return false
			}
		}
	}
	return false
}

func (a *App) moveProto(delta int) {
	if len(a.protos) == 0 {
		return
	}
	n := a.protoSel + delta
	if n < 0 {
		n = 0
	}
	if n >= len(a.protos) {
		n = len(a.protos) - 1
	}
	if n == a.protoSel {
		return
	}
	a.protoSel = n
	if a.protoSel < a.protoTop {
		a.protoTop = a.protoSel
	} else if a.protoSel >= a.protoTop+listRows {
		a.protoTop = a.protoSel - listRows + 1
	}
	a.paintProto()
}

func (a *App) paintProto() {
	title := "Download protocol"
	if a.protoUp {
		title = "Upload protocol"
	}
	a.scr.rule(1, chTL, chTR, title)
	label := ""
	if a.protoMsg {
		label = " Upload your message text"
	} else if a.protoUp {
		label = " Upload into " + a.uploadDir
	} else {
		names := make([]string, 0, len(a.protoFiles))
		for _, p := range a.protoFiles {
			names = append(names, filepath.Base(p))
		}
		label = " " + strings.Join(names, ", ")
	}
	a.scr.content(2, attrNorm, fit(label, contentWidth))
	for row := 0; row < listRows; row++ {
		i := a.protoTop + row
		text := blank(contentWidth)
		color := attrNorm
		if i >= 0 && i < len(a.protos) {
			p := a.protos[i]
			mark := " "
			if p.Key >= 32 {
				mark = string(p.Key)
			}
			text = fit(" "+mark+"  "+p.Name, contentWidth)
			if i == a.protoSel {
				color = attrBar
			}
		}
		a.scr.content(listY+row, color, text)
	}
	a.scr.content(22, attrNorm, blank(contentWidth))
	a.scr.content(statusY, attrNorm, a.statusText())
	a.scr.rule(24, chBL, chBR, "Up/Dn  Enter or key  Esc Back")
}

func (a *App) runProto(i int) {
	if i < 0 || i >= len(a.protos) {
		return
	}
	p := a.protos[i]
	env := a.xferEnv()
	if a.protoMsg {
		a.runMsgUpload(p, env)
		return
	}
	if a.protoUp {
		a.runUpload(p, env)
		return
	}
	if err := writeControl(p, a.protoFiles, env); err != nil {
		a.note = err.Error()
		a.mode = modeRead
		a.paintAll()
		return
	}
	line := expandDownload(p.DnCmd, env)
	if strings.TrimSpace(p.CtlFile) == "" {
		for _, file := range a.protoFiles {
			line += " " + quoteArg(file)
		}
	}
	back := a.lendCaller(true)
	defer back()
	err := runExternal(line, a.workDir(), a.user.Handle, false)
	setBlocking(a.port, true)
	a.mode = modeRead
	if err != nil {
		a.note = p.Name + " stopped: " + err.Error()
	} else {
		a.note = "Transfer finished (" + p.Name + ")."
	}
	a.paintAll()
}

func (a *App) runUpload(p Protocol, env xferEnv) {
	if err := writeUpControl(p, a.uploadDir, env); err != nil {
		a.abandonUpload()
		a.replyNote = err.Error()
		a.sendReply()
		return
	}
	line := expandUpload(p.UpCmd, a.uploadDir, env)
	a.eraseProtocolFiles(p, env)
	back := a.lendCaller(true)
	defer back()
	err := runExternal(line, a.workDir(), a.user.Handle, false)
	a.pullLogged(p, env)
	a.eraseProtocolFiles(p, env)
	setBlocking(a.port, true)
	extra := ""
	if len(dirFiles(a.uploadDir)) == 0 {
		a.abandonUpload()
		if err != nil {
			extra = p.Name + " stopped: " + err.Error()
		} else {
			extra = "No file received."
		}
	}
	a.protoUp = false
	a.sendReply()
	if extra != "" {
		a.note = extra
		a.paintAll()
	}
}

func (a *App) pullLogged(p Protocol, env xferEnv) {
	var text string
	for _, path := range a.logPaths(p, env) {
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		text += string(b) + "\n"
	}
	for _, name := range loggedNames(text, p.UpLog, p.NameWord) {
		a.moveIntoUpload(name)
	}
}

func (a *App) logPaths(p Protocol, env xferEnv) []string {
	var out []string
	seen := map[string]bool{}
	add := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		key := strings.ToLower(path)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, path)
	}
	name := strings.TrimSpace(expandDownload(p.LogFile, env))
	if name == "" {
		name = "dszlog"
	}
	if filepath.IsAbs(name) {
		add(name)
	} else {
		add(filepath.Join(a.workDir(), name))
		add(filepath.Join(a.uploadDir, name))
	}
	add(filepath.Join(a.uploadDir, "dszlog"))
	add(filepath.Join(a.workDir(), "dszlog"))
	return out
}

func (a *App) moveIntoUpload(name string) {
	name = strings.Trim(name, `"'`)
	if name == "" || protocolFile(name) {
		return
	}
	var candidates []string
	if filepath.IsAbs(name) {
		candidates = []string{name}
	} else {
		candidates = []string{
			filepath.Join(a.uploadDir, name),
			filepath.Join(a.workDir(), name),
			filepath.Join(a.attachDir(), name),
		}
	}
	for _, src := range candidates {
		st, err := os.Stat(src)
		if err != nil || st.IsDir() {
			continue
		}
		dst := filepath.Join(a.uploadDir, filepath.Base(src))
		if sameFile(src, dst) {
			return
		}
		if err := os.Rename(src, dst); err == nil {
			return
		}
		in, err := os.ReadFile(src)
		if err != nil {
			continue
		}
		if err := os.WriteFile(dst, in, 0644); err == nil {
			_ = os.Remove(src)
			return
		}
	}
}

func (a *App) eraseProtocolFiles(p Protocol, env xferEnv) {
	logName := strings.TrimSpace(expandDownload(p.LogFile, env))
	ctlName := strings.TrimSpace(expandDownload(p.CtlFile, env))
	if logName == "" {
		logName = "dszlog"
	}
	remove := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		_ = os.Remove(path)
	}
	if filepath.IsAbs(logName) {
		remove(logName)
	}
	if filepath.IsAbs(ctlName) {
		remove(ctlName)
	}
	for _, dir := range []string{a.uploadDir, a.workDir()} {
		if dir == "" {
			continue
		}
		remove(filepath.Join(dir, "dszlog"))
		remove(filepath.Join(dir, "dsz.log"))
		if base := filepath.Base(logName); base != "" && base != "." {
			remove(filepath.Join(dir, base))
		}
		if base := filepath.Base(ctlName); base != "" && base != "." {
			remove(filepath.Join(dir, base))
		}
	}
	if a.uploadDir == "" {
		return
	}
	logBase := strings.ToLower(filepath.Base(logName))
	ctlBase := strings.ToLower(filepath.Base(ctlName))
	ents, _ := os.ReadDir(a.uploadDir)
	for _, ent := range ents {
		name := strings.ToLower(ent.Name())
		if ent.IsDir() {
			continue
		}
		if protocolFile(name) || name == logBase || (ctlName != "" && name == ctlBase) {
			remove(filepath.Join(a.uploadDir, ent.Name()))
		}
	}
}

func (a *App) export(name string, body []byte) (string, error) {
	dir := filepath.Join(a.workDir(), "download")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, body, 0644); err != nil {
		return "", err
	}
	return path, nil
}

func (a *App) findAttach(name string) string {
	name = filepath.Base(name)
	candidates := []string{
		filepath.Join(a.attachDir(), name),
		filepath.Join(a.workDir(), name),
		filepath.Join(a.workDir(), "download", name),
	}
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

func stageAttach(dir, src string) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	base := filepath.Base(src)
	dst := filepath.Join(dir, base)
	if sameFile(src, dst) {
		return base, nil
	}
	in, err := os.ReadFile(src)
	if err != nil {
		return "", err
	}
	if st, err := os.Stat(dst); err == nil && !st.IsDir() {
		base = fileStamp(base)
		dst = filepath.Join(dir, base)
	}
	if err := os.WriteFile(dst, in, 0644); err != nil {
		return "", err
	}
	return base, nil
}

func sameFile(a, b string) bool {
	aa, err1 := filepath.Abs(a)
	bb, err2 := filepath.Abs(b)
	return err1 == nil && err2 == nil && strings.EqualFold(aa, bb)
}

func fileStamp(name string) string {
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	return stem + "-" + time.Now().Format("150405") + ext
}
