//go:build (freebsd || netbsd) && 386

package unix

import "syscall"

func PosixFallocate(fd int, off int64, size int64) error {
	r1, _, _ := syscall.Syscall6(posixFallocateTrap, uintptr(fd), uintptr(off), uintptr(off>>32), uintptr(size), uintptr(size>>32), 0)
	if r1 != 0 {
		return syscall.Errno(r1)
	}
	return nil
}
