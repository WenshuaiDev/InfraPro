//go:build linux

package main

import (
	"golang.org/x/sys/unix"
	"runtime"
	"syscall"
	"unsafe"
)

// Each invocation has its own address-space ceiling, in addition to the GC's
// smaller heap budget. Network syscalls are denied by the kernel for all threads.
func hardLimits() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if e := syscall.Setrlimit(syscall.RLIMIT_AS, &syscall.Rlimit{Cur: 2 << 30, Max: 2 << 30}); e != nil {
		return e
	}
	if _, _, e := syscall.RawSyscall6(syscall.SYS_PRCTL, 38, 1, 0, 0, 0, 0); e != 0 {
		return e
	}
	// BPF loads seccomp_data.nr, returns EPERM for networking, allows other
	// syscalls required by the Go runtime; read-only mounts contain filesystem I/O.
	filter := []syscall.SockFilter{{Code: 0x20, K: 0}}
	for _, nr := range []uint32{syscall.SYS_SOCKET, syscall.SYS_CONNECT, syscall.SYS_BIND, syscall.SYS_LISTEN, syscall.SYS_ACCEPT, syscall.SYS_SENDTO, syscall.SYS_RECVFROM} {
		filter = append(filter, syscall.SockFilter{Code: 0x15, Jf: 1, K: nr}, syscall.SockFilter{Code: 0x06, K: 0x00050000 | uint32(syscall.EPERM)})
	}
	filter = append(filter, syscall.SockFilter{Code: 0x06, K: 0x7fff0000})
	program := syscall.SockFprog{Len: uint16(len(filter)), Filter: &filter[0]}
	_, _, e := syscall.RawSyscall6(unix.SYS_SECCOMP, 1, 1, uintptr(unsafe.Pointer(&program)), 0, 0, 0)
	if e != 0 {
		return e
	}
	return nil
}
