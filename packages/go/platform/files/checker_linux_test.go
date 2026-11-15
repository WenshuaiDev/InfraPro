//go:build linux

package files

import (
	"golang.org/x/sys/unix"
	"os"
	"os/exec"
	"testing"
)

func TestCheckerLinuxKernelRestrictions(t *testing.T) {
	if os.Getenv("FILES_SANDBOX_PROBE") == "1" {
		if e := restrictCheckerChild(); e != nil {
			t.Fatal(e)
		}
		fd, e := unix.Socket(unix.AF_INET, unix.SOCK_STREAM, 0)
		if e == nil {
			unix.Close(fd)
			t.Fatal("network socket allowed")
		}
		if e != unix.EPERM {
			t.Fatal(e)
		}
		var r unix.Rlimit
		if e = unix.Getrlimit(unix.RLIMIT_FSIZE, &r); e != nil || r.Cur != 2<<20 {
			t.Fatal("file limit", e, r)
		}
		if e = unix.Getrlimit(unix.RLIMIT_NOFILE, &r); e != nil || r.Cur != 32 {
			t.Fatal("fd limit", e, r)
		}
		if _, e = os.Create("/forbidden-checker-output"); e == nil {
			t.Fatal("read-only filesystem bypass")
		}
		if os.Getuid() == 0 {
			t.Fatal("checker runs as root")
		}
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestCheckerLinuxKernelRestrictions$", "-test.v")
	cmd.Env = []string{"FILES_SANDBOX_PROBE=1", "GOMEMLIMIT=256MiB", "GOMAXPROCS=2"}
	out, e := cmd.CombinedOutput()
	if e != nil {
		t.Fatalf("restricted child %v: %s", e, out)
	}
	t.Log(string(out))
}
