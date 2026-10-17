//go:build (freebsd || netbsd) && arm

package unix

import "syscall"

func PosixFallocate(fd int, off int64, size int64) error {
	// ARM's calling convention inserts a padding register so the 64-bit
	// offset stays double-word aligned.
	r1, _, _ := syscall.Syscall6(posixFallocateTrap, uintptr(fd), 0, uintptr(off), uintptr(off>>32), uintptr(size), uintptr(size>>32))
	if r1 != 0 {
		return syscall.Errno(r1)
	}
	return nil
}
