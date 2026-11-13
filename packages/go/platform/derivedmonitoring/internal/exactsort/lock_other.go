//go:build !unix && !windows

package exactsort

import (
	"fmt"
	"os"
)

func lockFile(*os.File) error { return fmt.Errorf("temporary_lock_platform_unsupported") }
func busyLock(error) bool     { return false }
