//go:build unix

package exactsort

import (
	"errors"
	"os"
	"syscall"
)

func lockFile(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}
func busyLock(e error) bool { return errors.Is(e, syscall.EWOULDBLOCK) || errors.Is(e, syscall.EAGAIN) }
