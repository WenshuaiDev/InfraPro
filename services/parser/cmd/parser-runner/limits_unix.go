//go:build unix

package main

import "syscall"

func limitProcess() error {
	// Go reserves virtual address space; use a Go memory budget plus CPU, file
	// and descriptor hard limits. Deployment additionally enforces cgroup memory.
	for _, v := range []struct {
		kind  int
		value uint64
	}{{syscall.RLIMIT_CPU, 6}, {syscall.RLIMIT_NOFILE, 32}, {syscall.RLIMIT_FSIZE, 0}, {syscall.RLIMIT_CORE, 0}} {
		if e := syscall.Setrlimit(v.kind, &syscall.Rlimit{Cur: v.value, Max: v.value}); e != nil {
			return e
		}
	}
	return hardLimits()
}
