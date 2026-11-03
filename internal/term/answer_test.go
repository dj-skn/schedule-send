package term

import (
	"bytes"
	"io"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
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
		{"Ctrl-Space on an empty line does nothing", "\x00\r", "", Entered},
		{"Ctrl-Space with text does nothing", "ab\x00c\r", "abc", Entered},
		{"the end of the input", "abc", "abc", Ended},
	} {
		in := strings.NewReader(c.keys)
		var out bytes.Buffer
		line, outcome := edit(in, &out, "> ", 0)
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
	edit(strings.NewReader("ab\x1b[Dc\x03"), &out, "> ", 0)
	want := "\r> \x1b[K" + "\r> a\x1b[K" + "\r> ab\x1b[K" + "\r> ab\x1b[K\x1b[1D" + "\r> acb\x1b[K\x1b[1D" + "^C\n"
	if out.String() != want {
		t.Errorf("drew %q, want %q", out.String(), want)
	}
}

func TestEditLeavesTheNextAnswer(t *testing.T) {
	in := strings.NewReader("first\rsecond\r")
	var out bytes.Buffer
	for _, want := range []string{"first", "second"} {
		if line, _ := edit(in, &out, "", 0); line != want {
			t.Errorf("got %q, want %q", line, want)
		}
	}
}

func TestEditTellsAKeptEndOfInputFromCtrlSpace(t *testing.T) {
	// A NUL that was waiting when keys began to be read one by one is an end
	// of input Linux kept; one read after them was typed, as Ctrl-Space is.
	for _, c := range []struct {
		name, keys string
		queued     int
		line       string
		outcome    Outcome
	}{
		{"a kept end of input on an empty line", "\x00", 1, "", Ended},
		{"a kept end of input after typed-ahead text does nothing", "ab\x00\r", 3, "ab", Entered},
		{"Ctrl-Space after what was waiting does nothing", "a\x7f\x00\r", 1, "", Entered},
		{"without the count, a NUL ends an empty line", "\x00", -1, "", Ended},
		{"without the count, a NUL with text does nothing", "ab\x00\r", -1, "ab", Entered},
	} {
		line, outcome := edit(strings.NewReader(c.keys), io.Discard, "> ", c.queued)
		if line != c.line || outcome != c.outcome {
			t.Errorf("%s: got %q, %v; want %q, %v", c.name, line, outcome, c.line, c.outcome)
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
	restore, _, err := keyByKey(pty)
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

func TestEndOfInputBeforeKeyByKey(t *testing.T) {
	// script(1) sends an end of input as soon as its own input runs out,
	// which for a run fed nothing is before the first question switches the
	// terminal to keys. It must still end the answer, not leave it waiting.
	master, slave := openPtyPair(t)
	var tios syscall.Termios
	if err := termios(slave.Fd(), ioctlGetTermios, &tios); err != nil {
		t.Fatal(err)
	}
	if _, err := master.Write([]byte{tios.Cc[syscall.VEOF]}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond) // the terminal takes it while reading lines
	done := make(chan Outcome, 1)
	go func() {
		_, outcome := Answer(slave, "> ")
		done <- outcome
	}()
	select {
	case outcome := <-done:
		if outcome != Ended {
			t.Errorf("outcome %v, want Ended", outcome)
		}
	case <-time.After(5 * time.Second):
		master.Write([]byte("\r")) // let the reader go
		t.Fatal("the answer still waits after the end of input")
	}
}

func TestKeyByKeyCountsWhatIsWaiting(t *testing.T) {
	master, slave := openPtyPair(t)
	if _, err := master.Write([]byte("ab")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond) // the terminal takes it while reading lines
	restore, queued, err := keyByKey(slave)
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	if queued != 2 {
		t.Errorf("queued %d, want 2", queued)
	}
}

func TestCtrlSpaceOnATerminalDoesNothing(t *testing.T) {
	for _, c := range []struct{ keys, line string }{
		{"\x00\r", ""},
		{"\x00ab\x00\r", "ab"},
	} {
		master, slave := openPtyPair(t)
		type result struct {
			line    string
			outcome Outcome
		}
		done := make(chan result, 1)
		go func() {
			line, outcome := Answer(slave, "> ")
			done <- result{line, outcome}
		}()
		waitKeyByKey(t, slave)
		if _, err := master.Write([]byte(c.keys)); err != nil {
			t.Fatal(err)
		}
		select {
		case r := <-done:
			if r.line != c.line || r.outcome != Entered {
				t.Errorf("keys %q: got %q, %v; want %q, Entered", c.keys, r.line, r.outcome, c.line)
			}
		case <-time.After(5 * time.Second):
			master.Write([]byte("\r")) // let the reader go
			t.Fatalf("keys %q: no answer", c.keys)
		}
	}
}

// waitKeyByKey waits until f reads keys one by one, and a moment longer, so
// what is typed next comes after the switch counted what was waiting.
func waitKeyByKey(t *testing.T, f *os.File) {
	t.Helper()
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		var tios syscall.Termios
		if termios(f.Fd(), ioctlGetTermios, &tios) == nil && tios.Lflag&syscall.ICANON == 0 {
			time.Sleep(50 * time.Millisecond)
			return
		}
	}
	t.Fatal("the terminal never switched to reading keys")
}
