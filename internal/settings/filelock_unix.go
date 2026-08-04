//go:build !windows

// Package settings provides persistent file-based settings storage
// with OS-appropriate file locking.
package settings

import (
	"os"
	"syscall"
)

func lockFileExclusive(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_EX)
}

func unlockFileExclusive(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}
