//go:build !windows

package mobile

import "syscall"

// syncStorage writes every cached file through to storage.
func syncStorage() { syscall.Sync() }

// dupFD duplicates a file descriptor.
func dupFD(fd int) (int, error) { return syscall.Dup(fd) }
