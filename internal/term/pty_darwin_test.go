package term

import (
	"bytes"
	"os"
	"syscall"
	"testing"
	"unsafe"
)

// openPty opens a pseudo-terminal. On macOS only the slave end answers as
// one, so the master is granted and unlocked and its slave opened.
func openPty(t *testing.T) *os.File {
	t.Helper()
	_, slave := openPtyPair(t)
	return slave
}

// openPtyPair opens a pseudo-terminal's two ends: what is written to the
// master is typed on the slave.
func openPtyPair(t *testing.T) (master, slave *os.File) {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Skip("no pseudo-terminal:", err)
	}
	t.Cleanup(func() { master.Close() })
	var name [128]byte
	for _, call := range []struct{ request, arg uintptr }{
		{syscall.TIOCPTYGRANT, 0},
		{syscall.TIOCPTYUNLK, 0},
		{syscall.TIOCPTYGNAME, uintptr(unsafe.Pointer(&name[0]))},
	} {
		if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), call.request, call.arg); errno != 0 {
			t.Fatalf("pseudo-terminal ioctl %#x: %v", call.request, errno)
		}
	}
	slave, err = os.OpenFile(string(name[:bytes.IndexByte(name[:], 0)]), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { slave.Close() })
	return master, slave
}
