package term

import (
	"fmt"
	"os"
	"syscall"
	"testing"
	"unsafe"
)

// openPty opens a pseudo-terminal. On Linux its master end answers as one.
func openPty(t *testing.T) *os.File {
	t.Helper()
	f, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Skip("no pseudo-terminal:", err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

// openPtyPair opens a pseudo-terminal's two ends: what is written to the
// master is typed on the slave.
func openPtyPair(t *testing.T) (master, slave *os.File) {
	t.Helper()
	master = openPty(t)
	var unlock int32
	var n uint32
	for _, call := range []struct{ request, arg uintptr }{
		{syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&unlock))},
		{syscall.TIOCGPTN, uintptr(unsafe.Pointer(&n))},
	} {
		if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), call.request, call.arg); errno != 0 {
			t.Fatalf("pseudo-terminal ioctl %#x: %v", call.request, errno)
		}
	}
	slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", n), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { slave.Close() })
	return master, slave
}
