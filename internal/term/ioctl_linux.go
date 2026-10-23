package term

import "syscall"

// ioctlGetTermios and ioctlSetTermios are the requests that read and change a
// terminal's settings.
const (
	ioctlGetTermios = syscall.TCGETS
	ioctlSetTermios = syscall.TCSETS
)
