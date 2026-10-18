//go:build (freebsd || netbsd) && (amd64 || arm64 || riscv64)

package unix

import "syscall"

func PosixFallocate(fd int, off int64, size int64) error {
	r1, _, _ := syscall.Syscall(posixFallocateTrap, uintptr(fd), uintptr(off), uintptr(size))
	if r1 != 0 {
		return syscall.Errno(r1)
	}
	return nil
}
