package term

import (
	"os"
	"testing"
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
