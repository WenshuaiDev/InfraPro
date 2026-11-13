package monitoring

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"runtime/debug"
)

type BackupBuild struct {
	GoVersion      string `json:"go_version"`
	Module         string `json:"module"`
	SourceRevision string `json:"source_revision,omitempty"`
	Modified       *bool  `json:"source_modified,omitempty"`
	BinarySHA256   string `json:"binary_sha256"`
	BinaryBytes    int64  `json:"binary_bytes,string"`
}

func backupBuild(ctx context.Context) (*BackupBuild, error) {
	out := &BackupBuild{}
	if info, ok := debug.ReadBuildInfo(); ok {
		out.GoVersion = info.GoVersion
		out.Module = info.Main.Path
		for _, v := range info.Settings {
			switch v.Key {
			case "vcs.revision":
				out.SourceRevision = v.Value
			case "vcs.modified":
				// Test binaries and source-only container builds may not embed
				// VCS settings. Absence means unknown, not a clean checkout.
				if v.Value == "true" || v.Value == "false" {
					modified := v.Value == "true"
					out.Modified = &modified
				}
			}
		}
	}
	path, e := os.Executable()
	if e != nil {
		return nil, fmt.Errorf("backup build identity unavailable")
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, fmt.Errorf("backup build identity unavailable")
	}
	defer f.Close()
	h := sha256.New()
	size, e := io.Copy(h, f)
	if e != nil {
		return nil, fmt.Errorf("backup build hash unavailable")
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	out.BinarySHA256 = hex.EncodeToString(h.Sum(nil))
	out.BinaryBytes = size
	return out, nil
}
