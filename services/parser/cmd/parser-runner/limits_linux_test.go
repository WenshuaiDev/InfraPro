//go:build linux

package main

import (
	"golang.org/x/sys/unix"
	"runtime"
	"sync"
	"syscall"
	"testing"
)

func TestLinuxInvocationHardLimits(t *testing.T) {
	if e := limitProcess(); e != nil {
		t.Fatal(e)
	}
	for _, limit := range []struct {
		kind int
		max  uint64
	}{{syscall.RLIMIT_AS, 2 << 30}, {syscall.RLIMIT_CPU, 6}, {syscall.RLIMIT_NOFILE, 32}, {syscall.RLIMIT_FSIZE, 0}, {syscall.RLIMIT_CORE, 0}} {
		var v syscall.Rlimit
		if e := syscall.Getrlimit(limit.kind, &v); e != nil || v.Cur != limit.max || v.Max != limit.max {
			t.Fatalf("limit %d: %+v %v", limit.kind, v, e)
		}
	}
	if mem, e := unix.Mmap(-1, 0, 3<<30, unix.PROT_NONE, unix.MAP_PRIVATE|unix.MAP_ANON); e == nil {
		unix.Munmap(mem)
		t.Fatal("address-space ceiling bypassed")
	}
	var wg sync.WaitGroup
	fail := make(chan error, 8)
	for range 8 {
		wg.Go(func() {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			fd, e := unix.Socket(unix.AF_INET, unix.SOCK_STREAM, 0)
			if e == nil {
				unix.Close(fd)
			}
			if e != syscall.EPERM {
				fail <- e
			}
		})
	}
	wg.Wait()
	close(fail)
	for e := range fail {
		t.Fatalf("network isolation missing on OS thread: %v", e)
	}
	fds := []int{}
	for range 40 {
		fd, e := unix.Open("/dev/null", unix.O_RDONLY, 0)
		if e == syscall.EMFILE {
			break
		}
		if e != nil {
			t.Fatal(e)
		}
		fds = append(fds, fd)
	}
	for _, fd := range fds {
		unix.Close(fd)
	}
	if len(fds) >= 32 {
		t.Fatal("descriptor ceiling bypassed")
	}
	t.Log("kernel address-space/CPU/file/descriptor limits and TSYNC network denial verified")
}
