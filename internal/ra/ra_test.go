package ra

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"elereader/internal/jam"
)

func TestOffsets(t *testing.T) {
	if exitUser != 241 {
		t.Fatalf("user info offset %d", exitUser)
	}
	if exitMsgArea != 1194 {
		t.Fatalf("msg area offset %d", exitMsgArea)
	}
	if userMsgArea+63 != userBytes {
		// MsgArea through the end of the RA 2.50 user record is 63 bytes.
		t.Fatalf("user tail %d", userBytes-userMsgArea)
	}
}

func TestCurrentArea(t *testing.T) {
	dir := t.TempDir()
	node := filepath.Join(dir, "node1")
	sys := filepath.Join(dir, "bbs")
	if err := os.Mkdir(node, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(sys, 0755); err != nil {
		t.Fatal(err)
	}

	exitinfo := make([]byte, exitMsgArea+2)
	binary.LittleEndian.PutUint16(exitinfo[exitMsgArea:], 2)
	binary.LittleEndian.PutUint16(exitinfo[exitSecurity:], 100)
	exitinfo[exitFlags] = 0x01
	if err := os.WriteFile(filepath.Join(node, "EXITINFO.BBS"), exitinfo, 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ELEBBS", sys)
	t.Setenv("RA", "")

	cfg := make([]byte, cfgAddrOff+cfgAddrLen)
	putPascal(cfg[cfgMsgBaseOff:], filepath.Join(sys, "msg"))
	putPascal(cfg[cfgAttachOff:], `C:\bbs\attach`)
	putPascal(cfg[cfgEditorOff:], `C:\bbs\editor.exe %1`)
	binary.LittleEndian.PutUint16(cfg[cfgAddrOff:], 1)
	binary.LittleEndian.PutUint16(cfg[cfgAddrOff+2:], 2)
	binary.LittleEndian.PutUint16(cfg[cfgAddrOff+4:], 3)
	if err := os.WriteFile(filepath.Join(sys, "CONFIG.RA"), cfg, 0644); err != nil {
		t.Fatal(err)
	}

	msgs := make([]byte, msgRecBytes*2)
	putArea(msgs[0:], 1, "General", `msg\general`)
	putArea(msgs[msgRecBytes:], 2, "EleBBS", `C:\bbs\msg\elebbs`)
	msgs[msgRecBytes+msgTypeOff] = 1
	binary.LittleEndian.PutUint16(msgs[msgRecBytes+msgSysopSecOff:], 100)
	msgs[msgRecBytes+msgSysopFlagsOff] = 0x01
	if err := os.WriteFile(filepath.Join(sys, "MESSAGES.RA"), msgs, 0644); err != nil {
		t.Fatal(err)
	}

	area, err := Current(node)
	if err != nil {
		t.Fatal(err)
	}
	if !area.SysopAccess {
		t.Fatal("security 100 with flag A1 should have sysop access")
	}
	msgs[msgRecBytes+msgSysopNotFlagsOff] = 0x01
	if err := os.WriteFile(filepath.Join(sys, "MESSAGES.RA"), msgs, 0644); err != nil {
		t.Fatal(err)
	}
	if a, _ := Current(node); a.SysopAccess {
		t.Fatal("a SysopNotFlags bit the user has must deny sysop access")
	}
	if area.Name != "EleBBS" || area.Path != `C:\bbs\msg\elebbs` {
		t.Fatalf("current %+v", area)
	}
	if area.Kind != jam.AreaNetmail || area.Origin != "1:2/3" {
		t.Fatalf("kind %v origin %q", area.Kind, area.Origin)
	}
	if area.Editor != `C:\bbs\editor.exe %1` || area.Attach != `C:\bbs\attach` || area.Node != node || !area.AllowAttach {
		t.Fatalf("editor %q attach %q node %q allow %v", area.Editor, area.Attach, area.Node, area.AllowAttach)
	}
}

func TestAkasBBS(t *testing.T) {
	sys := t.TempDir()
	cfg := make([]byte, cfgTimeoutOff+2)
	binary.LittleEndian.PutUint16(cfg[cfgAddrOff:], 1)
	binary.LittleEndian.PutUint16(cfg[cfgAddrOff+2:], 340)
	binary.LittleEndian.PutUint16(cfg[cfgAddrOff+4:], 1101)
	if err := os.WriteFile(filepath.Join(sys, "CONFIG.RA"), cfg, 0644); err != nil {
		t.Fatal(err)
	}
	akas := []byte{0xE7, 0x03, 0x2C, 0x01, 0x02, 0, 0, 0, 0x26, 0, 0xF7, 0x01, 0x1E, 0, 0, 0}
	if err := os.WriteFile(filepath.Join(sys, "akas.bbs"), akas, 0644); err != nil {
		t.Fatal(err)
	}
	binary.LittleEndian.PutUint16(cfg[cfgTimeoutOff:], 400)
	if err := os.WriteFile(filepath.Join(sys, "CONFIG.RA"), cfg, 0644); err != nil {
		t.Fatal(err)
	}
	if got := readConfig(sys).idle; got != 400 {
		t.Fatalf("UserTimeOut %d", got)
	}
	addrs := readConfig(sys).addrs
	if got := addressAt(addrs, 0); got != "1:340/1101" {
		t.Fatal(got)
	}
	if got := addressAt(addrs, 10); got != "999:300/2" {
		t.Fatal(got)
	}
	if got := addressAt(addrs, 11); got != "38:503/30" {
		t.Fatal(got)
	}
}

func TestMessagesFromRA(t *testing.T) {
	sys := t.TempDir()
	if err := os.WriteFile(filepath.Join(sys, "MESSAGES.RA"), []byte{0}, 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ELEBBS", "")
	t.Setenv("RA", sys+`\`)
	got, err := findSystem()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(got, sys) {
		t.Fatalf("system dir %s", got)
	}
}

func putArea(b []byte, num uint16, name, jam string) {
	binary.LittleEndian.PutUint16(b[0:], num)
	b[msgAttrOff] = attrJAM | attrAttach
	putPascal(b[msgNameOff:msgNameOff+msgNameLen], name)
	putPascal(b[msgJamOff:msgJamOff+msgJamLen], jam)
}

func putPascal(b []byte, s string) {
	if len(s) >= len(b) {
		s = s[:len(b)-1]
	}
	b[0] = byte(len(s))
	copy(b[1:], s)
}
