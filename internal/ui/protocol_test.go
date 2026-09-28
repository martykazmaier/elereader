package ui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProtocol(t *testing.T) {
	dir := t.TempDir()
	rec := make([]byte, protocolBytes)
	putPascal(rec[0:16], "Zmodem")
	rec[16] = 'Z'
	rec[19] = 1
	putPascal(rec[182:263], `c:\ra\dsz.com port *P speed *B sz #`)
	putPascal(rec[101:182], `c:\ra\node*N\dsz.ctl`)
	putPascal(rec[263:344], "@")
	putPascal(rec[344:425], `c:\ra\dsz.com port *P speed *B rz #`)
	putPascal(rec[425:506], "@")
	if err := os.WriteFile(filepath.Join(dir, "PROTOCOL.RA"), rec, 0644); err != nil {
		t.Fatal(err)
	}
	got, err := loadProtocols(filepath.Join(dir, "PROTOCOL.RA"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "Zmodem" || got[0].Key != 'Z' {
		t.Fatalf("%+v", got)
	}
	env := xferEnv{baud: "38400", port: "1", node: "1", mins: "45", handle: "384"}
	line := expandDownload(got[0].DnCmd, env)
	if line != `c:\ra\dsz.com port 1 speed 38400 sz ` {
		t.Fatalf("%q", line)
	}
	if got[0].UpCmd != `c:\ra\dsz.com port *P speed *B rz #` {
		t.Fatalf("upload %q", got[0].UpCmd)
	}
	up := t.TempDir()
	ctl := filepath.Join(up, "dsz.ctl")
	got[0].CtlFile = ctl
	if err := writeUpControl(got[0], filepath.Join(up, "in"), env); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(ctl)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != filepath.Join(up, "in")+"\\\r\n" {
		t.Fatalf("%q", b)
	}
}

func TestDSZLogSkipsLog(t *testing.T) {
	log := "H  45056 38400 bps 3100 cps 0 errors  0  NOTES.ZIP  0\r\n"
	got := loggedNames(log, "H", 10)
	if len(got) != 1 || got[0] != "NOTES.ZIP" {
		t.Fatalf("%v", got)
	}
	for _, name := range []string{"dszlog", "DSZLOG", "dszlog.1", "DSZLOG.TXT", `c:\ele\node1\dsz.log`} {
		if !protocolFile(name) {
			t.Fatalf("%s should not be treated as the upload", name)
		}
	}
	if protocolFile("notes.txt") {
		t.Fatal("notes.txt is an upload")
	}
}

func TestUploadDropsLogs(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"DSZLOG.1", "xfer.log", "msg.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	a := &App{uploadDir: dir, areas: []Area{{Node: dir}}}
	a.eraseProtocolFiles(Protocol{LogFile: `c:\somewhere\XFER.LOG`}, xferEnv{})
	got := dirFiles(dir)
	if len(got) != 1 || filepath.Base(got[0]) != "msg.txt" {
		t.Fatalf("left %v", got)
	}
}

func TestAttachDir(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "n1")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.zip"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	got := filesIn(dir, root)
	if len(got) != 1 || filepath.Base(got[0]) != "notes.zip" {
		t.Fatalf("%v", got)
	}
	if filesIn(dir, filepath.Join(root, "other")) != nil {
		t.Fatal("path outside the attach directory was accepted")
	}
}

func putPascal(b []byte, s string) {
	if len(s) >= len(b) {
		s = s[:len(b)-1]
	}
	b[0] = byte(len(s))
	copy(b[1:], s)
}
