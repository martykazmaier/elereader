package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"elereader/internal/jam"
)

const protocolBytes = 550

// Protocol is one enabled download entry from PROTOCOL.RA.
type Protocol struct {
	Name     string
	Key      byte
	Opus     bool
	LogFile  string
	CtlFile  string
	DnCmd    string
	DnCtl    string
	UpCmd    string
	UpCtl    string
	UpLog    string
	NameWord byte
}

func (a *App) protocolPath() string {
	var dirs []string
	if sys := strings.TrimSpace(a.current().Sys); sys != "" {
		dirs = append(dirs, sys)
	}
	for _, key := range []string{"ELEBBS", "RA"} {
		dir := strings.Trim(strings.TrimSpace(os.Getenv(key)), `"'`)
		dir = strings.TrimRight(dir, `\/`)
		if dir != "" {
			dirs = append(dirs, dir)
		}
	}
	for _, dir := range dirs {
		for _, name := range []string{"PROTOCOL.RA", "protocol.ra"} {
			path := filepath.Join(dir, name)
			if st, err := os.Stat(path); err == nil && !st.IsDir() {
				return path
			}
		}
	}
	return ""
}

func loadProtocols(path string) ([]Protocol, error) {
	if path == "" {
		return nil, fmt.Errorf("PROTOCOL.RA not found in ELEBBS or RA")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(b) == 0 || len(b)%protocolBytes != 0 {
		return nil, fmt.Errorf("%s is not a PROTOCOL.RA file", path)
	}
	var out []Protocol
	for off := 0; off+protocolBytes <= len(b); off += protocolBytes {
		rec := b[off : off+protocolBytes]
		if rec[19] == 0 {
			continue
		}
		p := Protocol{
			Name:     pascalField(rec[0:16]),
			Key:      rec[16],
			Opus:     rec[17] != 0,
			LogFile:  pascalField(rec[20:101]),
			CtlFile:  pascalField(rec[101:182]),
			DnCmd:    pascalField(rec[182:263]),
			DnCtl:    pascalField(rec[263:344]),
			UpCmd:    pascalField(rec[344:425]),
			UpCtl:    pascalField(rec[425:506]),
			UpLog:    pascalField(rec[506:527]),
			NameWord: rec[549],
		}
		if p.Name == "" || (strings.TrimSpace(p.DnCmd) == "" && strings.TrimSpace(p.UpCmd) == "") {
			continue
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("PROTOCOL.RA has no external protocol commands")
	}
	return out, nil
}

func pascalField(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	n := int(b[0])
	if n >= len(b) {
		n = len(b) - 1
	}
	return strings.TrimRight(string(b[1:1+n]), " \x00")
}

type xferEnv struct {
	baud   string
	port   string
	node   string
	mins   string
	first  string
	last   string
	handle string
}

func (a *App) xferEnv() xferEnv {
	mins, unlimited := a.user.Remaining(time.Now())
	left := "1440"
	if !unlimited {
		left = itoa(uint32(mins))
	}
	first, last := splitName(a.user.RealName)
	if first == "" {
		first, last = splitName(a.user.Who())
	}
	return xferEnv{
		baud:   itoa(uint32(a.user.Baud)),
		port:   "1",
		node:   itoa(uint32(a.user.Node)),
		mins:   left,
		first:  first,
		last:   last,
		handle: itoa(a.user.Handle),
	}
}

func splitName(s string) (string, string) {
	f := strings.Fields(s)
	if len(f) == 0 {
		return "", ""
	}
	if len(f) == 1 {
		return f[0], ""
	}
	return f[0], f[len(f)-1]
}

func expandDownload(cmd string, env xferEnv) string {
	return expandCommand(cmd, env, "")
}

func expandUpload(cmd, dir string, env xferEnv) string {
	return expandCommand(cmd, env, forceBack(dir))
}

func forceBack(dir string) string {
	dir = strings.TrimRight(strings.TrimSpace(dir), `\/`)
	if dir == "" {
		return ""
	}
	return dir + `\`
}

func expandCommand(cmd string, env xferEnv, hash string) string {
	var b strings.Builder
	for i := 0; i < len(cmd); i++ {
		if cmd[i] == '#' {
			b.WriteString(hash)
			continue
		}
		if cmd[i] != '*' || i+1 >= len(cmd) {
			b.WriteByte(cmd[i])
			continue
		}
		switch cmd[i+1] {
		case 'B', 'b':
			b.WriteString(env.baud)
		case 'P', 'p':
			b.WriteString(env.port)
		case 'N', 'n':
			b.WriteString(env.node)
		case 'T', 't':
			b.WriteString(env.mins)
		case 'F', 'f':
			b.WriteString(env.first)
		case 'L', 'l':
			b.WriteString(env.last)
		case 'G', 'g':
			b.WriteString("1")
		case 'W', 'w':
			b.WriteString("@@HANDLE@@")
		case 'C', 'c':
			b.WriteString(os.Getenv("COMSPEC"))
		case 'H', 'h', 'Y', 'y', 'Z', 'z', '!', 'A', 'a', 'D', 'd', 'M', 'm', 'V', 'v', 'X', 'x':
		default:
			b.WriteByte('*')
			b.WriteByte(cmd[i+1])
		}
		i++
	}
	return b.String()
}

func writeControl(p Protocol, files []string, env xferEnv) error {
	name := strings.TrimSpace(expandDownload(p.CtlFile, env))
	if name == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil && filepath.Dir(name) != "." {
		return err
	}
	var b strings.Builder
	if p.Opus {
		fmt.Fprintf(&b, "Port %s\r\nBaud %s\r\nLog %s\r\nTime %s\r\n", env.port, env.baud, expandDownload(p.LogFile, env), env.mins)
	}
	line := p.DnCtl
	if strings.TrimSpace(line) == "" {
		line = "@"
	}
	for _, file := range files {
		b.WriteString(strings.ReplaceAll(expandDownload(line, env), "@", file))
		b.WriteString("\r\n")
	}
	return os.WriteFile(name, []byte(b.String()), 0644)
}

func writeUpControl(p Protocol, dir string, env xferEnv) error {
	name := strings.TrimSpace(expandDownload(p.CtlFile, env))
	if name == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil && filepath.Dir(name) != "." {
		return err
	}
	var b strings.Builder
	if p.Opus {
		fmt.Fprintf(&b, "Port %s\r\nBaud %s\r\nLog %s\r\nTime %s\r\n", env.port, env.baud, expandDownload(p.LogFile, env), env.mins)
	}
	line := p.UpCtl
	if strings.TrimSpace(line) == "" {
		line = "@"
	}
	b.WriteString(strings.ReplaceAll(expandDownload(line, env), "@", forceBack(dir)))
	b.WriteString("\r\n")
	return os.WriteFile(name, []byte(b.String()), 0644)
}

func (a *App) attachPaths(h jam.Header) []string {
	if h.Private() && !a.forUser(h) {
		return nil
	}
	dir := strings.TrimSpace(h.Subject)
	if paths := filesIn(dir, a.current().Attach); len(paths) > 0 {
		return paths
	}
	var out []string
	for _, name := range h.Files {
		if p := a.findAttach(name); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (a *App) attachNames(h jam.Header) []string {
	paths := a.attachPaths(h)
	names := make([]string, 0, len(paths))
	for _, p := range paths {
		names = append(names, filepath.Base(p))
	}
	return names
}

func dirFiles(dir string) []string {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, ent := range ents {
		if ent.IsDir() || protocolFile(ent.Name()) {
			continue
		}
		out = append(out, filepath.Join(dir, ent.Name()))
	}
	return out
}

func protocolFile(name string) bool {
	switch strings.ToLower(filepath.Base(name)) {
	case "dszlog", "dsz.log":
		return true
	default:
		return false
	}
}

func loggedNames(text, key string, word byte) []string {
	var out []string
	seen := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if key != "" {
			at := indexFold(line, key)
			if at < 0 {
				continue
			}
			fields = strings.Fields(line[at:])
		} else if len(fields) == 0 || !receivedMark(fields[0]) {
			continue
		}
		name := pickLogName(fields, word)
		if name == "" || protocolFile(name) {
			continue
		}
		id := strings.ToLower(name)
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, name)
	}
	return out
}

func receivedMark(s string) bool {
	switch strings.ToUpper(s) {
	case "H", "R":
		return true
	default:
		return false
	}
}

func pickLogName(fields []string, word byte) string {
	if word > 0 && int(word) <= len(fields) {
		f := strings.Trim(fields[word-1], `"'`)
		if looksLikeFile(f) {
			return f
		}
	}
	for i := len(fields) - 1; i >= 0; i-- {
		f := strings.Trim(fields[i], `"'`)
		if looksLikeFile(f) {
			return f
		}
	}
	return ""
}

func looksLikeFile(s string) bool {
	base := filepath.Base(s)
	if base == "" || base == "." || base == ".." || protocolFile(base) {
		return false
	}
	switch strings.ToLower(base) {
	case "bps", "cps", "errors", "error":
		return false
	}
	if _, err := strconv.Atoi(base); err == nil {
		return false
	}
	return strings.Contains(base, ".") || strings.ContainsAny(s, `\/`)
}

func indexFold(s, part string) int {
	return strings.Index(strings.ToLower(s), strings.ToLower(part))
}

func filesIn(dir, root string) []string {
	dir = strings.TrimRight(strings.TrimSpace(dir), `\/`)
	if dir == "" || !underDir(dir, root) {
		return nil
	}
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		return nil
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, ent := range ents {
		if ent.IsDir() || protocolFile(ent.Name()) {
			continue
		}
		out = append(out, filepath.Join(dir, ent.Name()))
	}
	return out
}

func underDir(path, root string) bool {
	path = strings.TrimRight(filepath.Clean(path), `\/`)
	root = strings.TrimSpace(root)
	if root == "" {
		return true
	}
	root = strings.TrimRight(filepath.Clean(root), `\/`)
	if strings.EqualFold(path, root) {
		return true
	}
	return strings.HasPrefix(strings.ToLower(path), strings.ToLower(root)+string(os.PathSeparator))
}
