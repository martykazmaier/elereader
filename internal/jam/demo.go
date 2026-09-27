package jam

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"time"
)

// WriteDemo builds a small JAM area and returns its path without an extension.
func WriteDemo(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "general")
	type row struct {
		from, to, subj, body string
		attr                 uint32
		when                 int64
	}
	rows := []row{
		{"Sysop", "All", "Welcome to Elereader", "This is the full-screen lightbar reader for EleBBS.\r\nUp and down move the bar. Enter opens a message.\r\n", attrTypeEcho, time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC).Unix()},
		{"Martin", "All", "Lightbar keys", "The bar only repaints the row you left and the row you landed on.\r\nPage Up and Page Down move a screen at a time.\r\n", attrTypeEcho, time.Date(2026, 9, 12, 18, 30, 0, 0, time.UTC).Unix()},
		{"Sysop", "Martin", "Note from the console", "Private mail stays masked unless it is to you or from you.\r\n", attrPrivate | attrTypeLoc, time.Date(2026, 9, 20, 9, 15, 0, 0, time.UTC).Unix()},
		{"Echo", "All", "JAM base", "Messages are read from the JAM files EleBBS already keeps.\r\n.JHR headers, .JDT text, .JDX index, .JLR lastread.\r\n", attrTypeEcho, time.Date(2026, 9, 26, 21, 4, 0, 0, time.UTC).Unix()},
	}

	var texts []byte
	var headers []byte
	var index []byte
	for i, row := range rows {
		body := []byte(row.body)
		subs := append(field(subSender, row.from), field(subReceiver, row.to)...)
		subs = append(subs, field(subSubject, row.subj)...)
		h := make([]byte, fixedLen+len(subs))
		binary.LittleEndian.PutUint32(h[0:], sigJAM)
		binary.LittleEndian.PutUint16(h[4:], 1)
		binary.LittleEndian.PutUint32(h[8:], uint32(len(subs)))
		binary.LittleEndian.PutUint32(h[36:], uint32(row.when))
		binary.LittleEndian.PutUint32(h[48:], uint32(i+1))
		binary.LittleEndian.PutUint32(h[52:], row.attr)
		binary.LittleEndian.PutUint32(h[60:], uint32(len(texts)))
		binary.LittleEndian.PutUint32(h[64:], uint32(len(body)))
		copy(h[fixedLen:], subs)
		off := 1024 + len(headers)
		rec := make([]byte, 8)
		binary.LittleEndian.PutUint32(rec[0:], CRC32String(row.to))
		binary.LittleEndian.PutUint32(rec[4:], uint32(off))
		index = append(index, rec...)
		headers = append(headers, h...)
		texts = append(texts, body...)
	}

	jhr := make([]byte, 1024)
	binary.LittleEndian.PutUint32(jhr[0:], sigJAM)
	binary.LittleEndian.PutUint32(jhr[12:], uint32(len(rows)))
	binary.LittleEndian.PutUint32(jhr[16:], 0xFFFFFFFF)
	binary.LittleEndian.PutUint32(jhr[20:], 1)
	jhr = append(jhr, headers...)

	if err := os.WriteFile(path+".JHR", jhr, 0644); err != nil {
		return "", err
	}
	if err := os.WriteFile(path+".JDT", texts, 0644); err != nil {
		return "", err
	}
	if err := os.WriteFile(path+".JDX", index, 0644); err != nil {
		return "", err
	}
	return path, nil
}

func field(id uint16, s string) []byte {
	b := make([]byte, 8+len(s))
	binary.LittleEndian.PutUint16(b[0:], id)
	binary.LittleEndian.PutUint32(b[4:], uint32(len(s)))
	copy(b[8:], s)
	return b
}
