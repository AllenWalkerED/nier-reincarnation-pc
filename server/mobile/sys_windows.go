//go:build windows

package mobile

import (
	"errors"
	"syscall"
)

// syncStorage: Windows has no system-wide sync; the archive import (the
// phone app's) is not used on Windows, where each file is closed after it is
// written.
func syncStorage() {}

// dupFD duplicates a handle of this process (an fd from the phone app has no
// meaning on Windows; the PC launcher never passes one).
func dupFD(fd int) (int, error) {
	p, e := syscall.GetCurrentProcess()
	if e != nil {
		return 0, e
	}
	var dup syscall.Handle
	if e = syscall.DuplicateHandle(p, syscall.Handle(fd), p, &dup, 0, false, syscall.DUPLICATE_SAME_ACCESS); e != nil {
		return 0, errors.Join(errors.New("duplicate handle"), e)
	}
	return int(dup), nil
}
