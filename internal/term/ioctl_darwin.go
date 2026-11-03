package term

import "syscall"

// ioctlGetTermios and ioctlSetTermios are the requests that read and change a
// terminal's settings, and ioctlQueued the one that counts the bytes waiting
// to be read.
const (
	ioctlGetTermios = syscall.TIOCGETA
	ioctlSetTermios = syscall.TIOCSETA
	// FIONREAD, _IOR('f', 127, int), which package syscall leaves out on
	// macOS.
	ioctlQueued = 0x4004667f
)
