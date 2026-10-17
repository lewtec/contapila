//go:build netbsd

package unix

// 479 is SYS_POSIX_FALLOCATE on NetBSD, matching Go's internal/syscall/unix.
const posixFallocateTrap uintptr = 479
