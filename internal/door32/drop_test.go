package door32

import (
	"strings"
	"testing"
	"time"
)

const sample = "2\r\n" +
	"384\r\n" +
	"38400\r\n" +
	"EleBBS 0.10\r\n" +
	"7\r\n" +
	"Martin\r\n" +
	"Martin\r\n" +
	"100\r\n" +
	"45\r\n" +
	"1\r\n" +
	"1\r\n"

func TestParseDoor32(t *testing.T) {
	d, err := ParseReader("DOOR32.SYS", strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	if d.CommType != CommTelnet || d.Handle != 384 {
		t.Fatalf("comm=%d handle=%d", d.CommType, d.Handle)
	}
	if d.BBSID != "EleBBS 0.10" || d.RealName != "Martin" || d.Alias != "Martin" {
		t.Fatalf("%+v", d)
	}
	if d.TimeLeft != 45 || d.Node != 1 || !d.ANSI() {
		t.Fatalf("%+v", d)
	}
	if d.Who() != "Martin" {
		t.Fatal(d.Who())
	}
}

func TestRemaining(t *testing.T) {
	d := Drop{TimeLeft: 10, Started: time.Now().Add(-3 * time.Minute)}
	mins, unlimited := d.Remaining(time.Now())
	if unlimited || mins != 7 {
		t.Fatalf("mins=%d unlimited=%v", mins, unlimited)
	}
	d.TimeLeft = 0
	_, unlimited = d.Remaining(time.Now())
	if !unlimited {
		t.Fatal("expected unlimited")
	}
}
