//go:build freebsd

package unix

import "syscall"

const posixFallocateTrap uintptr = syscall.SYS_POSIX_FALLOCATE
