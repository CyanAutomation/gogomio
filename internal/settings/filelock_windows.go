//go:build windows

package settings

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = kernel32.NewProc("LockFileEx")
	procUnlockFileEx = kernel32.NewProc("UnlockFileEx")
)

const lockfileExclusiveLock = 0x00000002

func lockFileExclusive(file *os.File) error {
	var ol syscall.Overlapped
	r1, _, err := procLockFileEx.Call(
		file.Fd(),
		uintptr(lockfileExclusiveLock),
		0,
		0xffffffff,
		0xffffffff,
		uintptr(unsafe.Pointer(&ol)),
	)
	if r1 == 0 {
		if err != syscall.Errno(0) {
			return err
		}
		return syscall.EINVAL
	}
	return nil
}

func unlockFileExclusive(file *os.File) error {
	var ol syscall.Overlapped
	r1, _, err := procUnlockFileEx.Call(
		file.Fd(),
		0,
		0xffffffff,
		0xffffffff,
		uintptr(unsafe.Pointer(&ol)),
	)
	if r1 == 0 {
		if err != syscall.Errno(0) {
			return err
		}
		return syscall.EINVAL
	}
	return nil
}
