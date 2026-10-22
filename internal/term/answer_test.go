package term

import (
	"bytes"
	"io"
	"strings"
	"syscall"
	"testing"
)

func TestEdit(t *testing.T) {
	for _, c := range []struct {
		name, keys, line string
		outcome          Outcome
	}{
		{"typed", "+30 min\r", "+30 min", Entered},
		{"Enter as a newline", "hi\n", "hi", Entered},
		{"inserted after Left", "helo\x1b[Dl\r", "hello", Entered},
		{"Backspace", "abc\x7f\x7fx\r", "ax", Entered},
		{"Backspace at the start", "\x7fa\r", "a", Entered},
		{"Delete under the cursor", "abc\x1b[D\x1b[D\x1b[3~\r", "ac", Entered},
		{"Home and End", "bc\x1b[Ha\x1b[Fd\r", "abcd", Entered},
		{"Home and End as Ctrl-A and Ctrl-E", "bc\x01a\x05d\r", "abcd", Entered},
		{"Home and End in the other forms", "bc\x1bOHa\x1b[4~d\x1b[1~_\r", "_abcd", Entered},
		{"Right stops at the end", "ab\x1b[C\x1b[Cc\r", "abc", Entered},
		{"Ctrl-U clears", "wrong\x15right\r", "right", Entered},
		{"Up, Down and Tab do nothing", "+3\x1b[A0\x1b[B\t\r", "+30", Entered},
		{"beyond ASCII", "zażółć\r", "zażółć", Entered},
		{"Ctrl-C", "ab\x03more\r", "ab", Cancelled},
		{"Ctrl-D on an empty line", "\x04", "", Ended},
		{"Ctrl-D with text does nothing", "ab\x04\r", "ab", Entered},
		{"the end of the input", "abc", "abc", Ended},
	} {
		in := strings.NewReader(c.keys)
		var out bytes.Buffer
		line, outcome := edit(in, &out, "> ")
		if line != c.line || outcome != c.outcome {
			t.Errorf("%s: got %q, %v; want %q, %v", c.name, line, outcome, c.line, c.outcome)
		}
		// The keys' own codes never reach the screen.
		if strings.Contains(out.String(), "[A") || strings.Contains(out.String(), "[3~") {
			t.Errorf("%s: drew %q", c.name, out.String())
		}
	}
}

func TestEditDraws(t *testing.T) {
	// Each change redraws the line, then steps back to the cursor; Ctrl-C
	// leaves the line as typed, marked.
	var out bytes.Buffer
	edit(strings.NewReader("ab\x1b[Dc\x03"), &out, "> ")
	want := "\r> \x1b[K" + "\r> a\x1b[K" + "\r> ab\x1b[K" + "\r> ab\x1b[K\x1b[1D" + "\r> acb\x1b[K\x1b[1D" + "^C\n"
	if out.String() != want {
		t.Errorf("drew %q, want %q", out.String(), want)
	}
}

func TestEditLeavesTheNextAnswer(t *testing.T) {
	in := strings.NewReader("first\rsecond\r")
	var out bytes.Buffer
	for _, want := range []string{"first", "second"} {
		if line, _ := edit(in, &out, ""); line != want {
			t.Errorf("got %q, want %q", line, want)
		}
	}
}

func TestClean(t *testing.T) {
	for in, want := range map[string]string{
		"\x1b[A\x1b[C+30":    "+30",
		"+3\x1b[1;5D0 min\t": "+30 min",
		"\x1bOHzażółć\x7f":   "zażółć",
		"16:30":              "16:30",
	} {
		if got := Clean(in); got != want {
			t.Errorf("Clean(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestKeyByKeyAndBack(t *testing.T) {
	pty := openPty(t)
	var before, during, after syscall.Termios
	if err := termios(pty.Fd(), ioctlGetTermios, &before); err != nil {
		t.Fatal(err)
	}
	restore, err := keyByKey(pty)
	if err != nil {
		t.Fatal(err)
	}
	termios(pty.Fd(), ioctlGetTermios, &during)
	restore()
	termios(pty.Fd(), ioctlGetTermios, &after)
	if during.Lflag&(syscall.ICANON|syscall.ECHO|syscall.ISIG) != 0 {
		t.Errorf("still reading lines, echoing, or signalling: lflag %#x", during.Lflag)
	}
	if during.Oflag != before.Oflag {
		t.Errorf("output processing changed: %#x, was %#x", during.Oflag, before.Oflag)
	}
	if after != before {
		t.Errorf("settings not put back:\n%+v\nwas\n%+v", after, before)
	}
}

func TestAnswerReadsLinesFromAnythingElse(t *testing.T) {
	var out bytes.Buffer
	tty := struct {
		io.Reader
		io.Writer
	}{strings.NewReader("\x1b[A+30\n"), &out}
	if line, outcome := Answer(tty, "> "); line != "+30" || outcome != Entered {
		t.Errorf("got %q, %v", line, outcome)
	}
	if out.String() != "> " {
		t.Errorf("drew %q", out.String())
	}
}
