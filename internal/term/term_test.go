package term

import (
	"bytes"
	"io"
	"os"
	"strings"
	"syscall"
	"testing"
	"unsafe"
)

func TestIsTerminal(t *testing.T) {
	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	if IsTerminal(null.Fd()) {
		t.Error("/dev/null counted as a terminal")
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if IsTerminal(w.Fd()) {
		t.Error("a pipe counted as a terminal")
	}
	if !IsTerminal(openPty(t).Fd()) {
		t.Error("a pseudo-terminal did not count")
	}
}

func TestColorsFor(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	if c := ColorsFor(&bytes.Buffer{}); c != (Colors{}) {
		t.Errorf("colours for a buffer: %q", c)
	}
	pty := openPty(t)
	if c := ColorsFor(pty); c.Reset != "\x1b[0m" || c.Green != "\x1b[32m" {
		t.Errorf("colours for a terminal: %q", c)
	}
	t.Setenv("NO_COLOR", "1")
	if c := ColorsFor(pty); c != (Colors{}) {
		t.Errorf("colours under NO_COLOR: %q", c)
	}
}

func TestReadLine(t *testing.T) {
	r := strings.NewReader("2\n16:30\npartial")
	for _, want := range []struct {
		line string
		ok   bool
	}{{"2", true}, {"16:30", true}, {"partial", false}, {"", false}} {
		line, ok := ReadLine(r)
		if line != want.line || ok != want.ok {
			t.Errorf("ReadLine = %q, %v; want %q, %v", line, ok, want.line, want.ok)
		}
	}
}

// A later prompt reads the same terminal, so a line read must leave the rest
// of the input where it was.
func TestReadLineLeavesTheRest(t *testing.T) {
	r := strings.NewReader("2\n\n")
	if line, _ := ReadLine(r); line != "2" {
		t.Fatalf("first line %q", line)
	}
	rest, _ := io.ReadAll(r)
	if string(rest) != "\n" {
		t.Errorf("left %q, want %q", rest, "\n")
	}
}

func TestWidthReadsTheWindow(t *testing.T) {
	master, slave := openPtyPair(t)
	size := struct{ Rows, Cols, X, Y uint16 }{Rows: 24, Cols: 52}
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), syscall.TIOCSWINSZ, uintptr(unsafe.Pointer(&size))); errno != 0 {
		t.Fatal("setting the window size:", errno)
	}
	if got := Width(slave.Fd()); got != 52 {
		t.Errorf("Width of a 52-column window = %d", got)
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if got := Width(r.Fd()); got != 0 {
		t.Errorf("Width of a pipe = %d, want 0", got)
	}
}
