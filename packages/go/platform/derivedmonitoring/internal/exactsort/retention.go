package exactsort

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PruneOrphans removes only expired private sort directories whose process
// lock can be acquired. A live long-running sort remains protected beyond TTL.
func PruneOrphans(ctx context.Context, directory string, before time.Time) (uint64, error) {
	if directory == "" {
		directory = os.TempDir()
	}
	root, e := os.Open(directory)
	if os.IsNotExist(e) {
		return 0, nil
	}
	if e != nil {
		return 0, e
	}
	defer root.Close()
	var removed uint64
	for {
		entries, e := root.ReadDir(100)
		if e != nil && e != io.EOF {
			return removed, e
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return removed, err
			}
			if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "infrapro-exact-sort-") {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				return removed, err
			}
			if !info.ModTime().Before(before) {
				continue
			}
			path := filepath.Join(directory, entry.Name())
			marker := filepath.Join(path, ".active")
			info, err = os.Lstat(marker)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return removed, err
			}
			if !info.Mode().IsRegular() {
				continue
			}
			lock, err := os.OpenFile(marker, os.O_RDWR, 0)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return removed, err
			}
			err = lockFile(lock)
			if err != nil {
				lock.Close()
				if busyLock(err) {
					continue
				}
				return removed, err
			}
			lock.Close()
			if err = os.RemoveAll(path); err != nil {
				return removed, err
			}
			removed++
		}
		if e == io.EOF {
			return removed, nil
		}
	}
}
