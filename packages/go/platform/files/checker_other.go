//go:build !linux

package files

import "net"

// Local non-Linux development relies on the 0700 directory / 0600 socket and
// child environment/deadline. Linux deployment supplies the full OS isolation.
func checkerPeerAllowed(*net.UnixConn) bool { return true }
func restrictCheckerChild() error           { return nil }
