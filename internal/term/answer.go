package term

import (
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
	"unicode/utf8"
	"unsafe"
)

// Outcome is how reading an answer ended.
type Outcome int

const (
	Entered   Outcome = iota // Enter was pressed
	Cancelled                // Ctrl-C was pressed
	Ended                    // the input ended, or Ctrl-D was pressed on an empty line
)

// Answer draws prompt on tty and reads one answer typed after it.
//
// On a terminal it reads key by key and edits the line itself: typed text goes
// in at the cursor, Left and Right move it, Home and End (or Ctrl-A and
// Ctrl-E) jump to the ends, Backspace and Delete remove a character, and
// Ctrl-U clears the line. Other keys, such as Up and Down, do nothing, where
// the terminal's own line reading would echo them as escape codes. Anything
// else, such as a pipe, is read a line at a time with escape codes and
// control characters left out.
func Answer(tty io.ReadWriter, prompt string) (string, Outcome) {
	if f, ok := tty.(*os.File); ok {
		if restore, err := keyByKey(f); err == nil {
			defer restore()
			return edit(f, f, prompt)
		}
	}
	fmt.Fprint(tty, prompt)
	line, ok := ReadLine(tty)
	if !ok {
		return Clean(line), Ended
	}
	return Clean(line), Entered
}

// keyByKey switches f to handing over each key as it is pressed, without echo
// and without turning Ctrl-C into a signal, and returns what puts it back.
// Output is processed as before, so a newline still returns the carriage.
func keyByKey(f *os.File) (restore func(), err error) {
	var old syscall.Termios
	if err := termios(f.Fd(), ioctlGetTermios, &old); err != nil {
		return nil, err
	}
	t := old
	t.Lflag &^= syscall.ICANON | syscall.ECHO | syscall.ISIG | syscall.IEXTEN
	t.Iflag &^= syscall.ICRNL | syscall.IXON
	t.Cc[syscall.VMIN], t.Cc[syscall.VTIME] = 1, 0
	if err := termios(f.Fd(), ioctlSetTermios, &t); err != nil {
		return nil, err
	}
	return func() { _ = termios(f.Fd(), ioctlSetTermios, &old) }, nil
}

func termios(fd, request uintptr, t *syscall.Termios) error {
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, request, uintptr(unsafe.Pointer(t))); errno != 0 {
		return errno
	}
	return nil
}

// edit reads keys from in until Enter, Ctrl-C, or the end of the input, and
// after each change redraws prompt and the line on out, with the cursor in
// its place. The line has no wrapping to follow, as answers are short.
func edit(in io.Reader, out io.Writer, prompt string) (string, Outcome) {
	var line []rune
	at := 0 // the cursor, as an index into line
	draw := func() {
		s := "\r" + prompt + string(line) + "\x1b[K"
		if back := len(line) - at; back > 0 {
			s += fmt.Sprintf("\x1b[%dD", back)
		}
		io.WriteString(out, s)
	}
	draw()
	keys := keyReader{in}
	for {
		k, r, ok := keys.next()
		if !ok {
			io.WriteString(out, "\n")
			return string(line), Ended
		}
		switch k {
		case keyEnter:
			io.WriteString(out, "\n")
			return string(line), Entered
		case keyCancel:
			io.WriteString(out, "^C\n")
			return string(line), Cancelled
		case keyEOF:
			if len(line) == 0 {
				io.WriteString(out, "\n")
				return "", Ended
			}
			continue
		case keyRune:
			line = append(line[:at], append([]rune{r}, line[at:]...)...)
			at++
		case keyBackspace:
			if at == 0 {
				continue
			}
			line = append(line[:at-1], line[at:]...)
			at--
		case keyDelete:
			if at == len(line) {
				continue
			}
			line = append(line[:at], line[at+1:]...)
		case keyLeft:
			at = max(at-1, 0)
		case keyRight:
			at = min(at+1, len(line))
		case keyHome:
			at = 0
		case keyEnd:
			at = len(line)
		case keyClear:
			line, at = nil, 0
		default:
			continue
		}
		draw()
	}
}

type key int

const (
	keyNone key = iota // a key the line has no use for
	keyRune
	keyEnter
	keyCancel
	keyEOF
	keyBackspace
	keyDelete
	keyLeft
	keyRight
	keyHome
	keyEnd
	keyClear
)

// keyReader turns a terminal's bytes into keys. It reads a byte at a time, so
// nothing typed after an answer is taken from the next one.
type keyReader struct{ in io.Reader }

func (k keyReader) byte() (byte, bool) {
	var b [1]byte
	for {
		n, err := k.in.Read(b[:])
		if n == 1 {
			return b[0], true
		}
		if err != nil {
			return 0, false
		}
	}
}

// next is the next key, with the character when it types one. ok is false at
// the end of the input.
func (k keyReader) next() (key, rune, bool) {
	b, ok := k.byte()
	if !ok {
		return keyNone, 0, false
	}
	switch {
	case b == '\r' || b == '\n':
		return keyEnter, 0, true
	case b == 0x03:
		return keyCancel, 0, true
	case b == 0x04:
		return keyEOF, 0, true
	case b == 0x7f || b == 0x08:
		return keyBackspace, 0, true
	case b == 0x01:
		return keyHome, 0, true
	case b == 0x05:
		return keyEnd, 0, true
	case b == 0x02:
		return keyLeft, 0, true
	case b == 0x06:
		return keyRight, 0, true
	case b == 0x15:
		return keyClear, 0, true
	case b == 0x1b:
		return k.escape(), 0, true
	case b < 0x20:
		return keyNone, 0, true
	case b < utf8.RuneSelf:
		return keyRune, rune(b), true
	}
	// The first byte of a character beyond ASCII says how many follow it.
	buf := []byte{b}
	for !utf8.FullRune(buf) && len(buf) < utf8.UTFMax {
		c, ok := k.byte()
		if !ok {
			return keyNone, 0, false
		}
		buf = append(buf, c)
	}
	if r, _ := utf8.DecodeRune(buf); r != utf8.RuneError {
		return keyRune, r, true
	}
	return keyNone, 0, true
}

// escape reads the rest of a key that starts with Esc: "[" and its
// parameters up to a final byte, or "O" and one letter. Esc followed by
// anything else is a key the line has no use for.
func (k keyReader) escape() key {
	b, ok := k.byte()
	if !ok {
		return keyNone
	}
	switch b {
	case 'O':
		b, _ = k.byte()
		return map[byte]key{'C': keyRight, 'D': keyLeft, 'H': keyHome, 'F': keyEnd}[b]
	case '[':
		var params []byte
		for {
			b, ok = k.byte()
			if !ok {
				return keyNone
			}
			if b >= 0x40 && b <= 0x7e {
				break
			}
			params = append(params, b)
		}
		if b == '~' {
			return map[string]key{"1": keyHome, "7": keyHome, "4": keyEnd, "8": keyEnd, "3": keyDelete}[string(params)]
		}
		return map[byte]key{'C': keyRight, 'D': keyLeft, 'H': keyHome, 'F': keyEnd}[b]
	}
	return keyNone
}

// Clean leaves out escape codes and other control characters from a line
// read with the terminal's own echo, where arrow keys arrive as escape codes.
func Clean(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == 0x1b && i+1 < len(s) && s[i+1] == '[':
			i += 2
			for i < len(s) && (s[i] < 0x40 || s[i] > 0x7e) {
				i++
			}
		case c == 0x1b && i+1 < len(s) && s[i+1] == 'O':
			i += 2
		case c < 0x20 || c == 0x7f:
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}
