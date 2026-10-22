// Package term answers whether a file is a terminal and holds the colour codes
// used when it is.
package term

import (
	"io"
	"os"
	"syscall"
	"unsafe"
)

// IsTerminal reports whether fd is a terminal, the way isatty(3) does. A
// character-device check would also accept /dev/null.
func IsTerminal(fd uintptr) bool {
	var t syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, ioctlGetTermios, uintptr(unsafe.Pointer(&t)))
	return errno == 0
}

// Colors holds the escape codes for one output, all empty when it is not a
// terminal or NO_COLOR is set.
type Colors struct {
	Reset, Dim, Red, Green, Yellow, Cyan, Bold string
}

// ColorsFor returns the colours for w: on only for a terminal, and never
// under NO_COLOR.
func ColorsFor(w io.Writer) Colors {
	f, ok := w.(*os.File)
	if !ok || os.Getenv("NO_COLOR") != "" || !IsTerminal(f.Fd()) {
		return Colors{}
	}
	return Colors{
		Reset: "\x1b[0m", Dim: "\x1b[2m", Red: "\x1b[31m",
		Green: "\x1b[32m", Yellow: "\x1b[33m", Cyan: "\x1b[36m", Bold: "\x1b[1m",
	}
}

// ReadLine reads one line from r a byte at a time, so nothing past the newline
// is consumed from a terminal that a later prompt will read again. The newline
// is dropped. ok is false at end of input, as with the shell's read builtin,
// even when a partial line came before it.
func ReadLine(r io.Reader) (line string, ok bool) {
	var buf []byte
	b := make([]byte, 1)
	for {
		n, err := r.Read(b)
		if n == 1 {
			if b[0] == '\n' {
				return string(buf), true
			}
			buf = append(buf, b[0])
		}
		if err != nil {
			return string(buf), false
		}
	}
}
