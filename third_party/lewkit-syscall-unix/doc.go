// Package unix is the posix_fallocate helper imported by lewkit's copied
// Go linker (x/toolchain/go/src/link/ld). That module rewrites
// internal/syscall/unix to this path and does not publish the package,
// so go mod tidy cannot load packages that import the linker.
package unix
