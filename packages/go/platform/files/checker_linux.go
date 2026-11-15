//go:build linux

package files

import (
	"golang.org/x/sys/unix"
	"net"
	"os"
	"unsafe"
)

func checkerPeerAllowed(c *net.UnixConn) bool {
	raw, e := c.SyscallConn()
	if e != nil {
		return false
	}
	allowed := false
	if e = raw.Control(func(fd uintptr) {
		peer, err := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		allowed = err == nil && peer.Uid == uint32(os.Geteuid())
	}); e != nil {
		return false
	}
	return allowed
}
func restrictCheckerChild() error {
	if e := unix.Setrlimit(unix.RLIMIT_FSIZE, &unix.Rlimit{Cur: 2 << 20, Max: 2 << 20}); e != nil {
		return e
	}
	if e := unix.Setrlimit(unix.RLIMIT_NOFILE, &unix.Rlimit{Cur: 32, Max: 32}); e != nil {
		return e
	}
	if e := unix.Setrlimit(unix.RLIMIT_CPU, &unix.Rlimit{Cur: 15, Max: 16}); e != nil {
		return e
	}
	if e := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); e != nil {
		return e
	}
	// All socket creation in the parser child is forbidden. Its parent owns the
	// AF_UNIX connection; the child communicates only on inherited stdin/stdout.
	filter := []unix.SockFilter{
		{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS, K: 0},
		{Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: uint32(unix.SYS_SOCKET), Jf: 1},
		{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_ERRNO | uint32(unix.EPERM)},
		{Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_ALLOW},
	}
	program := unix.SockFprog{Len: uint16(len(filter)), Filter: &filter[0]}
	_, _, errno := unix.Syscall(unix.SYS_SECCOMP, unix.SECCOMP_SET_MODE_FILTER, unix.SECCOMP_FILTER_FLAG_TSYNC, uintptr(unsafe.Pointer(&program)))
	if errno != 0 {
		return errno
	}
	return nil
}
