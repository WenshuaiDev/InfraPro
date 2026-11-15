// Package securefile provides descriptor-relative, no-follow configuration IO.
// It never includes paths or file contents in errors exposed to its callers.
package securefile

import (
	"crypto/rand"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

var ErrUnsafe = errors.New("unsafe_file")
var ErrLimit = errors.New("file_limit_exceeded")
var ErrLocked = errors.New("file_locked")

type Policy struct {
	UID         int
	SharedGroup bool
	Public      bool
}

func Private() Policy { return Policy{UID: os.Getuid()} }

// directory walks each component using O_NOFOLLOW. The returned descriptor
// pins the actual parent even if an ancestor is renamed concurrently.
func directory(path string) (int, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return -1, ErrUnsafe
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, ErrUnsafe
	}
	for _, part := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		if part == "" {
			continue
		}
		next, e := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		unix.Close(fd)
		if e != nil {
			return -1, e
		}
		fd = next
		var st unix.Stat_t
		if unix.Fstat(fd, &st) != nil || (st.Mode&0022 != 0 && st.Mode&unix.S_ISVTX == 0) {
			unix.Close(fd)
			return -1, ErrUnsafe
		}
	}
	return fd, nil
}

func check(st *unix.Stat_t, p Policy, dir bool) error {
	typ := uint32(unix.S_IFREG)
	if dir {
		typ = unix.S_IFDIR
	}
	if uint32(st.Mode)&unix.S_IFMT != typ || int(st.Uid) != p.UID {
		return ErrUnsafe
	}
	mask := uint32(0077)
	if p.SharedGroup {
		mask = 0027
	}
	if p.Public {
		mask = 0022
	}
	if uint32(st.Mode)&mask != 0 {
		return ErrUnsafe
	}
	return nil
}

func Open(path string, p Policy) (*os.File, error) {
	return OpenMode(path, os.O_RDONLY, p)
}

// OpenMode is used by the local state store; callers may only request ordinary
// read/write/create flags. Truncation and following symlinks are never allowed.
func OpenMode(path string, flags int, p Policy) (*os.File, error) {
	if flags & ^(os.O_RDONLY|os.O_RDWR|os.O_CREATE|os.O_EXCL) != 0 {
		return nil, ErrUnsafe
	}
	fd, err := directory(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	defer unix.Close(fd)
	n, err := unix.Openat(fd, filepath.Base(path), flags|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0600)
	if err != nil {
		return nil, err
	}
	var st unix.Stat_t
	if unix.Fstat(n, &st) != nil || check(&st, p, false) != nil {
		unix.Close(n)
		return nil, ErrUnsafe
	}
	return os.NewFile(uintptr(n), "protected-file"), nil
}

func Read(path string, limit int64, p Policy) ([]byte, error) {
	f, err := Open(path, p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if len(data) > int(limit) {
		return nil, ErrLimit
	}
	return data, err
}

func CheckDirectory(path string, p Policy) error {
	fd, err := directory(path)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil {
		return ErrUnsafe
	}
	return check(&st, p, true)
}

// Atomic writes on the same filesystem, fsyncs, then renames through the pinned
// parent. Existing symlinks and unsafe targets are refused, never followed.
func Atomic(path string, data []byte, p Policy, exclusive bool) error {
	parent := filepath.Dir(path)
	if err := CheckDirectory(parent, p); err != nil {
		return err
	}
	fd, err := directory(parent)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	base := filepath.Base(path)
	var st unix.Stat_t
	err = unix.Fstatat(fd, base, &st, unix.AT_SYMLINK_NOFOLLOW)
	if err == nil {
		if exclusive {
			return os.ErrExist
		}
		if check(&st, p, false) != nil {
			return ErrUnsafe
		}
	} else if !errors.Is(err, unix.ENOENT) {
		return err
	}
	// O_EXCL and a random suffix prevent collisions between independent writers.
	name := ".diagnostic-" + rand.Text()
	tfd, err := unix.Openat(fd, name, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return err
	}
	tmp := os.NewFile(uintptr(tfd), "protected-temp")
	defer unix.Unlinkat(fd, name, 0)
	defer tmp.Close()
	mode := os.FileMode(0600)
	if p.SharedGroup {
		mode = 0640
	}
	if err = tmp.Chmod(mode); err != nil {
		return err
	}
	if _, err = tmp.Write(data); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if exclusive {
		err = unix.Linkat(fd, name, fd, base, 0)
	} else {
		err = unix.Renameat(fd, name, fd, base)
	}
	if err != nil {
		return err
	}
	return unix.Fsync(fd)
}

type Lock struct{ file *os.File }

func Acquire(path string) (*Lock, error) {
	if err := CheckDirectory(filepath.Dir(path), Private()); err != nil {
		return nil, err
	}
	fd, err := directory(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	defer unix.Close(fd)
	n, err := unix.Openat(fd, filepath.Base(path), unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return nil, err
	}
	var st unix.Stat_t
	if unix.Fstat(n, &st) != nil || check(&st, Private(), false) != nil {
		unix.Close(n)
		return nil, ErrUnsafe
	}
	if unix.Flock(n, unix.LOCK_EX|unix.LOCK_NB) != nil {
		unix.Close(n)
		return nil, ErrLocked
	}
	return &Lock{os.NewFile(uintptr(n), "protected-lock")}, nil
}
func (l *Lock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	return l.file.Close()
}

// Replace installs an already-fsynced private regular file in the same protected
// directory. Neither endpoint may be a symlink. Callers own their writer lock.
func Replace(temp, destination string) error {
	parent := filepath.Dir(destination)
	if filepath.Dir(temp) != parent || temp == destination || CheckDirectory(parent, Private()) != nil {
		return ErrUnsafe
	}
	fd, err := directory(parent)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	for _, name := range []string{filepath.Base(temp), filepath.Base(destination)} {
		var st unix.Stat_t
		if unix.Fstatat(fd, name, &st, unix.AT_SYMLINK_NOFOLLOW) != nil || check(&st, Private(), false) != nil {
			return ErrUnsafe
		}
	}
	if err = unix.Renameat(fd, filepath.Base(temp), fd, filepath.Base(destination)); err != nil {
		return err
	}
	return unix.Fsync(fd)
}
