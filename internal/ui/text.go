package ui

import (
	"fmt"
	"strings"
	"unicode"
)

const contentWidth = 78
const ansiWidth = 80
const maxANSIRows = 2000

// bodyLines draws the message the way a terminal would, then returns one
// string per screen row. Cursor movement and erase stay inside that
// buffer, so a message cannot clear the door.
func bodyLines(raw []byte, width int) []string {
	if width <= 0 {
		width = ansiWidth
	}
	c := &canvas{w: width, max: maxANSIRows, st: style{fg: 7}}
	c.write(filterMessage(raw))
	rows := c.rows
	for len(rows) > 0 && rowBlank(rows[len(rows)-1]) {
		rows = rows[:len(rows)-1]
	}
	out := make([]string, len(rows))
	for i, row := range rows {
		out[i] = string(emitRow(row))
	}
	return out
}

func splitRaw(raw []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i := 0; i < len(raw); i++ {
		if raw[i] == '\n' || raw[i] == 0x8A || raw[i] == 0x8D {
			end := i
			if end > start && raw[end-1] == '\r' {
				end--
			}
			lines = append(lines, raw[start:end])
			start = i + 1
		} else if raw[i] == '\r' {
			lines = append(lines, raw[start:i])
			if i+1 < len(raw) && raw[i+1] == '\n' {
				i++
			}
			start = i + 1
		}
	}
	if start < len(raw) {
		lines = append(lines, raw[start:])
	}
	return lines
}

func filterMessage(raw []byte) []byte {
	var out []byte
	for i := 0; i < len(raw); {
		if atLineStart(out) && (kludgeLine(raw[i:]) || noiseLine(raw[i:])) {
			i = skipLine(raw, i)
			continue
		}
		switch raw[i] {
		case 0x8D, 0x8A:
			out = append(out, '\n')
		case 0x01:
			out = append(out, '@')
		default:
			out = append(out, raw[i])
		}
		i++
	}
	return out
}

func atLineStart(out []byte) bool {
	return len(out) == 0 || out[len(out)-1] == '\n' || out[len(out)-1] == '\r'
}

func lineEnd(b []byte) int {
	for i := 0; i < len(b); i++ {
		if b[i] == '\n' || b[i] == '\r' || b[i] == 0x8A || b[i] == 0x8D {
			return i
		}
	}
	return len(b)
}

func skipLine(raw []byte, i int) int {
	i += lineEnd(raw[i:])
	if i < len(raw) && raw[i] == '\r' {
		i++
	}
	if i < len(raw) && (raw[i] == '\n' || raw[i] == 0x8A || raw[i] == 0x8D) {
		i++
	}
	return i
}

func kludgeLine(b []byte) bool {
	return len(b) > 0 && b[0] == 0x01
}

func noiseLine(b []byte) bool {
	end := lineEnd(b)
	plain := strings.TrimSpace(strings.ToUpper(string(stripSGR(b[:end]))))
	return strings.HasPrefix(plain, "SEEN-BY:") || strings.HasPrefix(plain, "PATH:")
}

type style struct {
	fg    byte
	bg    byte
	blink bool
}

type cell struct {
	ch byte
	st style
}

type canvas struct {
	w, max     int
	rows       [][]cell
	row, col   int
	st         style
	sRow, sCol int
	sSt        style
	last       byte
}

func (c *canvas) grow(row int) {
	if row < 0 || row >= c.max {
		return
	}
	for len(c.rows) <= row {
		line := make([]cell, c.w)
		def := style{fg: 7}
		for i := range line {
			line[i] = cell{ch: ' ', st: def}
		}
		c.rows = append(c.rows, line)
	}
}

func (c *canvas) put(ch byte) {
	if ch == 0 {
		ch = ' '
	}
	if c.col >= c.w {
		c.col = 0
		c.row++
	}
	if c.row < 0 {
		c.row = 0
	}
	if c.row >= c.max {
		return
	}
	c.grow(c.row)
	if c.row >= len(c.rows) || c.col < 0 || c.col >= c.w {
		return
	}
	c.rows[c.row][c.col] = cell{ch: ch, st: c.st}
	c.last = ch
	c.col++
}

func (c *canvas) move(row, col int) {
	if row < 1 {
		row = 1
	}
	if col < 1 {
		col = 1
	}
	c.row = row - 1
	c.col = col - 1
	if c.row >= c.max {
		c.row = c.max - 1
	}
	if c.col >= c.w {
		c.col = c.w - 1
	}
	c.grow(c.row)
}

func (c *canvas) write(b []byte) {
	for i := 0; i < len(b); {
		if b[i] == 0x1b {
			n := c.escape(b[i:])
			if n > 0 {
				i += n
				continue
			}
		}
		switch b[i] {
		case '\r':
			c.col = 0
		case '\n':
			c.row++
			c.col = 0
			if c.row >= c.max {
				c.row = c.max - 1
			}
		case '\t':
			n := 8 - (c.col % 8)
			if c.col < 0 {
				n = 8
			}
			for k := 0; k < n; k++ {
				c.put(' ')
			}
		default:
			if b[i] >= 0x20 {
				c.put(b[i])
			}
		}
		i++
	}
}

func (c *canvas) escape(b []byte) int {
	if len(b) < 2 || b[0] != 0x1b {
		return 0
	}
	switch b[1] {
	case '[':
		i := 2
		for i < len(b) && (b[i] < 0x40 || b[i] > 0x7e) {
			i++
		}
		if i >= len(b) {
			return len(b)
		}
		body := b[2:i]
		if !csiPrivate(body) {
			c.csi(b[i], csiNumbers(body))
		}
		return i + 1
	case ']':
		i := 2
		for i < len(b) && b[i] != 0x07 {
			if b[i] == 0x1b && i+1 < len(b) && b[i+1] == '\\' {
				return i + 2
			}
			i++
		}
		if i < len(b) {
			return i + 1
		}
		return len(b)
	case '(', ')', '*', '+':
		if len(b) >= 3 {
			return 3
		}
		return len(b)
	case '7':
		c.sRow, c.sCol, c.sSt = c.row, c.col, c.st
		return 2
	case '8':
		c.row, c.col, c.st = c.sRow, c.sCol, c.sSt
		return 2
	case 'c':
		c.st = style{fg: 7}
		c.row, c.col = 0, 0
		return 2
	default:
		return 2
	}
}

func csiPrivate(body []byte) bool {
	for _, ch := range body {
		switch ch {
		case '?', '>', '!', '=', '<':
			return true
		}
	}
	return false
}

func csiNumbers(body []byte) []int {
	if len(body) == 0 {
		return nil
	}
	var out []int
	cur := 0
	any := false
	for _, ch := range body {
		if ch >= '0' && ch <= '9' {
			cur = cur*10 + int(ch-'0')
			if cur > 9999 {
				cur = 9999
			}
			any = true
			continue
		}
		if ch == ';' {
			if !any {
				out = append(out, 0)
			} else {
				out = append(out, cur)
			}
			cur = 0
			any = false
		}
	}
	if !any {
		out = append(out, 0)
	} else {
		out = append(out, cur)
	}
	return out
}

func csiParam(params []int, i, def int) int {
	if i >= len(params) || params[i] == 0 {
		return def
	}
	return params[i]
}

func (c *canvas) csi(final byte, params []int) {
	switch final {
	case 'm':
		c.st = applySGR(c.st, params)
	case 'H', 'f':
		c.move(csiParam(params, 0, 1), csiParam(params, 1, 1))
	case 'A':
		c.row -= csiParam(params, 0, 1)
		if c.row < 0 {
			c.row = 0
		}
	case 'B':
		c.row += csiParam(params, 0, 1)
		if c.row >= c.max {
			c.row = c.max - 1
		}
	case 'C':
		c.col += csiParam(params, 0, 1)
		if c.col >= c.w {
			c.col = c.w - 1
		}
	case 'D':
		c.col -= csiParam(params, 0, 1)
		if c.col < 0 {
			c.col = 0
		}
	case 'E':
		c.row += csiParam(params, 0, 1)
		c.col = 0
		if c.row >= c.max {
			c.row = c.max - 1
		}
	case 'F':
		c.row -= csiParam(params, 0, 1)
		if c.row < 0 {
			c.row = 0
		}
		c.col = 0
	case 'G':
		c.col = csiParam(params, 0, 1) - 1
		if c.col < 0 {
			c.col = 0
		}
		if c.col >= c.w {
			c.col = c.w - 1
		}
	case 's':
		c.sRow, c.sCol, c.sSt = c.row, c.col, c.st
	case 'u':
		c.row, c.col, c.st = c.sRow, c.sCol, c.sSt
	case 'K':
		c.eraseLine(csiParam(params, 0, 0))
	case 'J':
		c.eraseDisplay(csiParam(params, 0, 0))
	case 'b':
		ch := c.last
		if ch == 0 {
			ch = ' '
		}
		n := csiParam(params, 0, 1)
		for k := 0; k < n; k++ {
			c.put(ch)
		}
	}
}

func (c *canvas) eraseLine(mode int) {
	if c.row < 0 || c.row >= c.max {
		return
	}
	c.grow(c.row)
	if c.row >= len(c.rows) {
		return
	}
	from, to := c.col, c.w
	switch mode {
	case 1:
		from, to = 0, c.col+1
	case 2:
		from, to = 0, c.w
	}
	if from < 0 {
		from = 0
	}
	if to > c.w {
		to = c.w
	}
	for i := from; i < to && i < len(c.rows[c.row]); i++ {
		c.rows[c.row][i] = cell{ch: ' ', st: c.st}
	}
}

func (c *canvas) eraseDisplay(mode int) {
	clear := func(row int) {
		if row < 0 || row >= len(c.rows) {
			return
		}
		for i := range c.rows[row] {
			c.rows[row][i] = cell{ch: ' ', st: c.st}
		}
	}
	switch mode {
	case 1:
		for i := 0; i < c.row; i++ {
			clear(i)
		}
		c.eraseLine(1)
	case 2:
		for i := range c.rows {
			clear(i)
		}
	default:
		c.eraseLine(0)
		for i := c.row + 1; i < len(c.rows); i++ {
			clear(i)
		}
	}
}

func applySGR(st style, params []int) style {
	if len(params) == 0 {
		return style{fg: 7}
	}
	for i := 0; i < len(params); i++ {
		p := params[i]
		switch {
		case p == 0:
			st = style{fg: 7}
		case p == 1:
			if st.fg < 8 {
				st.fg += 8
			}
		case p == 22:
			if st.fg >= 8 {
				st.fg -= 8
			}
		case p == 5:
			st.blink = true
		case p == 25:
			st.blink = false
		case p == 7:
			st.fg, st.bg = st.bg, st.fg
		case p >= 30 && p <= 37:
			bright := byte(0)
			if st.fg >= 8 {
				bright = 8
			}
			st.fg = bright + byte(p-30)
		case p >= 90 && p <= 97:
			st.fg = 8 + byte(p-90)
		case p >= 40 && p <= 47:
			st.bg = byte(p - 40)
		case p >= 100 && p <= 107:
			st.bg = 8 + byte(p-100)
		case p == 39:
			st.fg = 7
		case p == 49:
			st.bg = 0
		case p == 38 || p == 48:
			if i+1 < len(params) && params[i+1] == 5 {
				i += 2
			} else if i+1 < len(params) && params[i+1] == 2 {
				i += 4
			}
		}
	}
	return st
}

func styleANSI(st style) []byte {
	fg := st.fg
	bold := ""
	if fg >= 8 {
		bold = "1;"
		fg -= 8
	}
	bg := 40 + int(st.bg)
	if st.bg >= 8 {
		bg = 100 + int(st.bg-8)
	}
	blink := ""
	if st.blink {
		blink = "5;"
	}
	return []byte(fmt.Sprintf("\x1b[0;%s%s%d;%dm", blink, bold, 30+int(fg), bg))
}

func rowBlank(row []cell) bool {
	for _, c := range row {
		if c.ch != ' ' && c.ch != 0 {
			return false
		}
		if c.st.fg != 7 || c.st.bg != 0 || c.st.blink {
			return false
		}
	}
	return true
}

func emitRow(row []cell) []byte {
	var out []byte
	cur := style{fg: 7}
	for _, c := range row {
		if c.st != cur {
			out = append(out, styleANSI(c.st)...)
			cur = c.st
		}
		ch := c.ch
		if ch == 0 {
			ch = ' '
		}
		out = append(out, ch)
	}
	return out
}

func keepSGR(line []byte) []byte {
	var out []byte
	for i := 0; i < len(line); {
		if line[i] == 0x1b {
			seq, n := consumeESC(line[i:])
			if n > 0 {
				if isSGR(seq) {
					out = append(out, seq...)
				}
				i += n
				continue
			}
		}
		if line[i] < 0x20 && line[i] != '\t' {
			i++
			continue
		}
		out = append(out, line[i])
		i++
	}
	return out
}

func stripSGR(line []byte) []byte {
	var out []byte
	for i := 0; i < len(line); {
		if line[i] == 0x1b {
			_, n := consumeESC(line[i:])
			if n > 0 {
				i += n
				continue
			}
		}
		if line[i] < 0x20 && line[i] != '\t' {
			i++
			continue
		}
		out = append(out, line[i])
		i++
	}
	return out
}

func consumeESC(b []byte) ([]byte, int) {
	if len(b) == 0 || b[0] != 0x1b {
		return nil, 0
	}
	if len(b) == 1 {
		return b[:1], 1
	}
	if b[1] != '[' {
		return b[:2], 2
	}
	i := 2
	for i < len(b) {
		if b[i] >= 0x40 && b[i] <= 0x7e {
			return b[:i+1], i + 1
		}
		i++
	}
	return b, len(b)
}

func isSGR(seq []byte) bool {
	return len(seq) >= 3 && seq[0] == 0x1b && seq[1] == '[' && seq[len(seq)-1] == 'm'
}

func wrapVisible(line []byte, width int) []string {
	if len(line) == 0 {
		return []string{""}
	}
	var out []string
	var cur []byte
	vis := 0
	flush := func() {
		out = append(out, string(cur))
		cur = cur[:0]
		vis = 0
	}
	for i := 0; i < len(line); {
		if line[i] == 0x1b {
			seq, n := consumeESC(line[i:])
			if n > 0 && isSGR(seq) {
				cur = append(cur, seq...)
				i += n
				continue
			}
		}
		if line[i] == '\t' {
			line[i] = ' '
		}
		if vis >= width {
			flush()
		}
		cur = append(cur, line[i])
		vis++
		i++
	}
	if len(cur) > 0 || len(out) == 0 {
		out = append(out, string(cur))
	}
	return out
}

func padVisible(line string, width int) []byte {
	b := []byte(line)
	var out []byte
	vis := 0
	for i := 0; i < len(b) && vis < width; {
		if b[i] == 0x1b {
			seq, n := consumeESC(b[i:])
			if n > 0 && isSGR(seq) {
				out = append(out, seq...)
				i += n
				continue
			}
		}
		out = append(out, b[i])
		vis++
		i++
	}
	for vis < width {
		out = append(out, ' ')
		vis++
	}
	return out
}

func fit(s string, n int) []byte {
	b := stripSGR([]byte(s))
	if len(b) > n {
		b = b[:n]
	}
	out := make([]byte, n)
	copy(out, b)
	for i := len(b); i < n; i++ {
		out[i] = ' '
	}
	return out
}

func blank(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = ' '
	}
	return b
}

func clipTitle(s string, n int) string {
	b := []byte(s)
	if len(b) > n {
		b = b[:n]
	}
	return string(b)
}

// slide moves a lightbar inside a fixed window. The window scrolls only
// when the selection would leave it.
func slide(sel, top, height, count, dest int) (int, int) {
	if count <= 0 {
		return 0, 0
	}
	if dest < 0 {
		dest = 0
	}
	if dest >= count {
		dest = count - 1
	}
	sel = dest
	if sel < top {
		top = sel
	} else if sel >= top+height {
		top = sel - height + 1
	}
	if top < 0 {
		top = 0
	}
	return sel, top
}

func eqName(a, b string) bool {
	return a != "" && b != "" && strings.EqualFold(strings.TrimFunc(a, unicode.IsSpace), strings.TrimFunc(b, unicode.IsSpace))
}
