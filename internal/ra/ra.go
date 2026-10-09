// Package ra reads the RemoteAccess files EleBBS still writes.
// Words are 16-bit and records are packed, matching the RA 2.50 file format.
package ra

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"elereader/internal/jam"
	"elereader/internal/ui"
)

const (
	userBytes = 1016
	// MsgArea is the uint16 at this offset inside USERSrecord.
	userMsgArea = 953
	// UserInfo begins after Baud, SYSINFO, and TIMELOG.
	exitUser = 2 + 168 + 71
	// Absolute offset of the current message area in EXITINFO.BBS.
	exitMsgArea = exitUser + userMsgArea
	// USERSrecord Flags (array[1..4] of Byte) and Security (word).
	exitFlags    = exitUser + 436
	exitSecurity = exitUser + 450

	// MESSAGErecord SysopSecurity, SysopFlags, SysopNotFlags.
	msgSysopSecOff      = 72
	msgSysopFlagsOff    = 74
	msgSysopNotFlagsOff = 78

	msgRecBytes = 224
	msgNameOff  = 4
	msgNameLen  = 41
	msgTypeOff  = 45
	msgAttrOff  = 47
	msgAkaOff   = 143
	msgJamOff   = 145
	msgJamLen   = 61
	attrJAM     = 0x80
	// MESSAGES.RA attribute bit 2 is file attaches (1 shl 2).
	attrAttach = 0x04

	// Pascal paths and the address list in CONFIG.RA.
	cfgAttachOff  = 873
	cfgAttachLen  = 61
	cfgMsgBaseOff = 995
	cfgMsgBaseLen = 61
	cfgEditorOff  = 1117
	cfgEditorLen  = 61
	cfgAddrOff    = 1178
	cfgAddrLen    = 8
	cfgAddrCount  = 10
	cfgTimeoutOff = 1479
)

// Current reads the caller's message area. The working directory is the
// node directory and holds EXITINFO.BBS. MESSAGES.RA and CONFIG.RA are
// in the ELEBBS directory, or the RA directory when ELEBBS is not set.
// CONFIG.RA supplies MsgBasePath for a relative JAM name. EleBBS changes
// areas before the door runs, so only this area is returned.
func Current(nodeDir string) (ui.Area, error) {
	if nodeDir == "" {
		nodeDir = "."
	}
	areaNum, user, err := messageArea(nodeDir)
	if err != nil {
		return ui.Area{}, err
	}
	sysDir, err := findSystem()
	if err != nil {
		return ui.Area{}, err
	}
	recs, err := readMessages(filepath.Join(sysDir, "MESSAGES.RA"))
	if err != nil {
		recs, err = readMessages(filepath.Join(sysDir, "messages.ra"))
		if err != nil {
			return ui.Area{}, err
		}
	}
	cfg := readConfig(sysDir)
	idle := idleSeconds(cfg)
	root := cfg.msgBase
	if root == "" {
		root = sysDir
	}
	for _, rec := range recs {
		if int(rec.num) != areaNum {
			continue
		}
		path := joinBase(root, rec.jam)
		if path == "" {
			return ui.Area{}, fmt.Errorf("message area %d has no JAM path", areaNum)
		}
		name := rec.name
		if name == "" {
			name = fmt.Sprintf("Area %d", rec.num)
		}
		return ui.Area{
			Name:        name,
			Path:        path,
			Kind:        areaKind(rec.typ),
			Origin:      addressAt(cfg.addrs, rec.aka),
			Editor:      cfg.editor,
			IdleSecs:    idle,
			Attach:      joinBase(sysDir, cfg.attach),
			AllowAttach: rec.attr&attrAttach != 0,
			SysopAccess: sysopAccess(user, rec),
			Node:        nodeDir,
			Sys:         sysDir,
		}, nil
	}
	return ui.Area{}, fmt.Errorf("message area %d is not a JAM area in MESSAGES.RA", areaNum)
}

type exitUserInfo struct {
	security uint16
	flags    [4]byte
}

func messageArea(nodeDir string) (int, exitUserInfo, error) {
	var user exitUserInfo
	for _, name := range []string{"EXITINFO.BBS", "exitinfo.bbs"} {
		b, err := os.ReadFile(filepath.Join(nodeDir, name))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return 0, user, err
		}
		if len(b) < exitMsgArea+2 {
			return 0, user, fmt.Errorf("%s is too small to hold the current message area", name)
		}
		n := int(binary.LittleEndian.Uint16(b[exitMsgArea:]))
		if n <= 0 {
			return 0, user, fmt.Errorf("%s has no current message area", name)
		}
		copy(user.flags[:], b[exitFlags:exitFlags+4])
		user.security = binary.LittleEndian.Uint16(b[exitSecurity:])
		return n, user, nil
	}
	return 0, user, fmt.Errorf("EXITINFO.BBS not found in %s", nodeDir)
}

// sysopAccess is EleBBS SysOpAccess: the user has every SysopFlags bit,
// none of the SysopNotFlags bits, and at least SysopSecurity.
func sysopAccess(user exitUserInfo, rec msgRec) bool {
	for i := 0; i < 4; i++ {
		if user.flags[i]&rec.sysopFlags[i] != rec.sysopFlags[i] {
			return false
		}
		if user.flags[i]&rec.sysopNotFlags[i] != 0 {
			return false
		}
	}
	return user.security >= rec.sysopSec
}

type msgRec struct {
	num           uint16
	name          string
	jam           string
	typ           byte
	aka           byte
	attr          byte
	sysopSec      uint16
	sysopFlags    [4]byte
	sysopNotFlags [4]byte
}

func readMessages(path string) ([]msgRec, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(b) == 0 || len(b)%msgRecBytes != 0 {
		return nil, fmt.Errorf("%s is %d bytes, not a multiple of the %d-byte RA area record", path, len(b), msgRecBytes)
	}
	var out []msgRec
	for off := 0; off+msgRecBytes <= len(b); off += msgRecBytes {
		rec := b[off : off+msgRecBytes]
		num := binary.LittleEndian.Uint16(rec[0:2])
		if num == 0 {
			continue
		}
		jam := raString(rec[msgJamOff : msgJamOff+msgJamLen])
		if rec[msgAttrOff]&attrJAM == 0 && jam == "" {
			continue
		}
		m := msgRec{
			num:      num,
			name:     raString(rec[msgNameOff : msgNameOff+msgNameLen]),
			jam:      jam,
			typ:      rec[msgTypeOff],
			aka:      rec[msgAkaOff],
			attr:     rec[msgAttrOff],
			sysopSec: binary.LittleEndian.Uint16(rec[msgSysopSecOff:]),
		}
		copy(m.sysopFlags[:], rec[msgSysopFlagsOff:msgSysopFlagsOff+4])
		copy(m.sysopNotFlags[:], rec[msgSysopNotFlagsOff:msgSysopNotFlagsOff+4])
		out = append(out, m)
	}
	return out, nil
}

func findSystem() (string, error) {
	var checked []string
	for _, key := range []string{"ELEBBS", "RA"} {
		dir := strings.Trim(strings.TrimSpace(os.Getenv(key)), `"'`)
		dir = strings.TrimRight(dir, `\/`)
		if dir == "" {
			continue
		}
		checked = append(checked, dir)
		if fileExists(filepath.Join(dir, "MESSAGES.RA")) || fileExists(filepath.Join(dir, "messages.ra")) {
			return dir, nil
		}
	}
	if len(checked) == 0 {
		return "", fmt.Errorf("MESSAGES.RA not found: ELEBBS and RA are not set")
	}
	return "", fmt.Errorf("MESSAGES.RA not found in %s", strings.Join(checked, " or "))
}

type bbsConfig struct {
	msgBase string
	attach  string
	editor  string
	addrs   []string
	idle    int
	found   bool
}

func readConfig(sysDir string) bbsConfig {
	var cfg bbsConfig
	for _, name := range []string{"CONFIG.RA", "config.ra"} {
		b, err := os.ReadFile(filepath.Join(sysDir, name))
		if err != nil {
			continue
		}
		cfg.msgBase = pascalAt(b, cfgMsgBaseOff, cfgMsgBaseLen)
		cfg.attach = pascalAt(b, cfgAttachOff, cfgAttachLen)
		cfg.editor = pascalAt(b, cfgEditorOff, cfgEditorLen)
		cfg.found = true
		if len(b) >= cfgTimeoutOff+2 {
			cfg.idle = int(binary.LittleEndian.Uint16(b[cfgTimeoutOff:]))
		}
		for i := 0; i < cfgAddrCount; i++ {
			off := cfgAddrOff + i*cfgAddrLen
			if off+cfgAddrLen > len(b) {
				break
			}
			cfg.addrs = append(cfg.addrs, formatAddr(b[off:off+cfgAddrLen]))
		}
		break
	}
	cfg.addrs = append(cfg.addrs, readAkas(sysDir, len(cfg.addrs))...)
	return cfg
}

// idleSeconds is UserTimeOut from CONFIG.RA in %RA%, or from the system
// directory when %RA% has none.
func idleSeconds(sys bbsConfig) int {
	dir := strings.TrimRight(strings.Trim(strings.TrimSpace(os.Getenv("RA")), `"'`), `\/`)
	if dir != "" {
		if cfg := readConfig(dir); cfg.found {
			return cfg.idle
		}
	}
	return sys.idle
}

// readAkas returns AKAS.BBS padded so AKA 10 is its first record. EleBBS
// takes AKAs 0-9 from CONFIG.RA and the rest from AKAS.BBS.
func readAkas(sysDir string, have int) []string {
	for _, name := range []string{"AKAS.BBS", "akas.bbs"} {
		b, err := os.ReadFile(filepath.Join(sysDir, name))
		if err != nil {
			continue
		}
		var out []string
		for i := have; i < cfgAddrCount; i++ {
			out = append(out, "")
		}
		for off := 0; off+cfgAddrLen <= len(b); off += cfgAddrLen {
			out = append(out, formatAddr(b[off:off+cfgAddrLen]))
		}
		return out
	}
	return nil
}

func pascalAt(b []byte, off, n int) string {
	if off < 0 || n <= 0 || off+n > len(b) {
		return ""
	}
	return raString(b[off : off+n])
}

func formatAddr(b []byte) string {
	if len(b) < 8 {
		return ""
	}
	zone := binary.LittleEndian.Uint16(b[0:2])
	net := binary.LittleEndian.Uint16(b[2:4])
	node := binary.LittleEndian.Uint16(b[4:6])
	point := binary.LittleEndian.Uint16(b[6:8])
	if zone == 0 && net == 0 && node == 0 && point == 0 {
		return ""
	}
	if point == 0 {
		return fmt.Sprintf("%d:%d/%d", zone, net, node)
	}
	return fmt.Sprintf("%d:%d/%d.%d", zone, net, node, point)
}

func addressAt(addrs []string, aka byte) string {
	if int(aka) < len(addrs) && addrs[aka] != "" {
		return addrs[aka]
	}
	if len(addrs) > 0 {
		return addrs[0]
	}
	return ""
}

func areaKind(typ byte) jam.AreaKind {
	switch typ {
	case 1:
		return jam.AreaNetmail
	case 2, 4:
		return jam.AreaEcho
	case 3:
		return jam.AreaEmail
	default:
		return jam.AreaLocal
	}
}

func joinBase(root, jam string) string {
	jam = strings.TrimSpace(jam)
	if jam == "" {
		return ""
	}
	if filepath.IsAbs(jam) || strings.Contains(jam, ":") || strings.HasPrefix(jam, `\`) || strings.HasPrefix(jam, "/") {
		return jam
	}
	return filepath.Join(root, jam)
}

func raString(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	n := int(b[0])
	if n > 0 && n < len(b) && textish(b[1:1+n]) {
		return clean(b[1 : 1+n])
	}
	return clean(cString(b))
}

func cString(b []byte) []byte {
	for i, c := range b {
		if c == 0 {
			return b[:i]
		}
	}
	return b
}

func textish(b []byte) bool {
	for _, c := range b {
		if c == 0 || c == '\t' {
			continue
		}
		if c < 32 {
			return false
		}
	}
	return len(b) > 0
}

func clean(b []byte) string {
	return strings.TrimRightFunc(string(b), unicode.IsSpace)
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}
