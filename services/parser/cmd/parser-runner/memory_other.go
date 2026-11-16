//go:build !linux

package main

// Non-Linux local validation uses the parent process's five-second kill boundary
// and the Go heap budget. Production uses the provided Linux container limits.
func hardLimits() error { return nil }
