package term

import "syscall"

// ioctlGetTermios and ioctlSetTermios are the requests that read and change a
// terminal's settings, and ioctlQueued the one that counts the bytes waiting
// to be read.
const (
	ioctlGetTermios = syscall.TCGETS
	ioctlSetTermios = syscall.TCSETS
	ioctlQueued     = syscall.TIOCINQ
)
