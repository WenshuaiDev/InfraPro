package securefile

import (
	"os"
	"path/filepath"
	"testing"
)

func root(t *testing.T) string {
	t.Helper()
	p, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	if e = os.Chmod(p, 0700); e != nil {
		t.Fatal(e)
	}
	return p
}
func TestAtomicAndNoFollow(t *testing.T) {
	dir := root(t)
	path := filepath.Join(dir, "auth.json")
	if e := Atomic(path, []byte("old"), Private(), true); e != nil {
		t.Fatal(e)
	}
	if e := Atomic(path, []byte("new"), Private(), true); e == nil {
		t.Fatal("exclusive overwrite")
	}
	if e := Atomic(path, []byte("new"), Private(), false); e != nil {
		t.Fatal(e)
	}
	got, e := Read(path, 8, Private())
	if e != nil || string(got) != "new" {
		t.Fatal("atomic value", e)
	}
	alias := filepath.Join(dir, "alias")
	if e = os.Symlink(path, alias); e != nil {
		t.Fatal(e)
	}
	if _, e = Read(alias, 8, Private()); e == nil {
		t.Fatal("followed file symlink")
	}
	if e = Atomic(alias, []byte("unsafe"), Private(), false); e == nil {
		t.Fatal("replaced symlink")
	}
	link := filepath.Join(dir, "linked")
	if e = os.Symlink(dir, link); e != nil {
		t.Fatal(e)
	}
	if _, e = Read(filepath.Join(link, "auth.json"), 8, Private()); e == nil {
		t.Fatal("followed parent symlink")
	}
	if e = os.Chmod(path, 0644); e != nil {
		t.Fatal(e)
	}
	if _, e = Read(path, 8, Private()); e == nil {
		t.Fatal("read public secret")
	}
}
func TestLockAndLimit(t *testing.T) {
	dir := root(t)
	lock, e := Acquire(filepath.Join(dir, "lock"))
	if e != nil {
		t.Fatal(e)
	}
	defer lock.Close()
	if other, e := Acquire(filepath.Join(dir, "lock")); e == nil {
		other.Close()
		t.Fatal("duplicate writer")
	}
	path := filepath.Join(dir, "data")
	if e = Atomic(path, []byte("12345"), Private(), true); e != nil {
		t.Fatal(e)
	}
	if _, e = Read(path, 4, Private()); e != ErrLimit {
		t.Fatal(e)
	}
}
