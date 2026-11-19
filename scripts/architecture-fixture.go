// This fixture command only operates on disposable architecture-test resources.
// It keeps passwords out of argv and reads objects back from both buckets.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	pruntime "github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type blob struct {
	Key  string `json:"key"`
	Size int64  `json:"size"`
	Hash string `json:"sha256"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) != 3 {
		return errors.New("usage: architecture-fixture seed|snapshot|restore|list DIRECTORY")
	}
	cfg, err := config.Load(os.Getenv("INFRAPRO_CONFIG"))
	if err != nil {
		return err
	}
	u, err := url.Parse(cfg.DatabaseURL)
	if err != nil || !strings.HasPrefix(u.Path, "/arc_") || !net.ParseIP(u.Hostname()).IsLoopback() {
		return errors.New("a dedicated loopback architecture database is required")
	}
	ctx := context.Background()
	dir := os.Args[2]
	if os.Args[1] == "seed" {
		raw, err := os.ReadFile(filepath.Join(dir, "accounts.json"))
		if err != nil {
			return err
		}
		var accounts map[string]struct{ Username, Password string }
		if err = json.Unmarshal(raw, &accounts); err != nil {
			return err
		}
		host, err := pruntime.Open(ctx, cfg, slog.Default(), true)
		if err != nil {
			return err
		}
		defer host.Close()
		user := accounts["admin"]
		_, err = host.Identity.Bootstrap(ctx, user.Username, "架构验收管理员", user.Password, identity.Meta{})
		return err
	}
	m := cfg.Monitoring
	endpoint, e := url.Parse(m.ObjectEndpoint)
	prefix := ""
	// The frozen SDK load fixture places only this synthetic tenant's objects
	// in the integration bucket. Limit snapshot access to that tenant prefix.
	if os.Args[1] == "snapshot" && m.Bucket == "contract-source" {
		id, err := uuid.Parse(os.Getenv("ARC_DISPLAY_SNAPSHOT_TENANT"))
		if err != nil {
			return errors.New("synthetic tenant UUID required for integration fixture")
		}
		prefix = id.String() + "/"
	}
	if e != nil || !net.ParseIP(endpoint.Hostname()).IsLoopback() || (!strings.HasPrefix(m.Bucket, "arc-") && prefix == "") {
		return errors.New("dedicated loopback object bucket required")
	}
	client, err := minio.New(endpoint.Host, &minio.Options{Creds: credentials.NewStaticV4(m.ObjectAccessKey, m.ObjectSecretKey, ""), BucketLookup: minio.BucketLookupPath})
	if err != nil {
		return err
	}
	if os.Args[1] == "restore" {
		b, err := os.ReadFile(filepath.Join(dir, "objects.json"))
		if err != nil {
			return err
		}
		var objects []blob
		if err = json.Unmarshal(b, &objects); err != nil {
			return err
		}
		for _, o := range objects {
			data, err := os.ReadFile(filepath.Join(dir, o.Hash+".blob"))
			if err != nil {
				return err
			}
			sum := sha256.Sum256(data)
			if hex.EncodeToString(sum[:]) != o.Hash || int64(len(data)) != o.Size {
				return errors.New("invalid snapshot digest")
			}
			if _, err = client.PutObject(ctx, m.Bucket, o.Key, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{}); err != nil {
				return err
			}
		}
	}
	if os.Args[1] != "restore" && os.Args[1] != "snapshot" && os.Args[1] != "list" {
		return errors.New("unknown fixture operation")
	}
	items := []blob{}
	for o := range client.ListObjects(ctx, m.Bucket, minio.ListObjectsOptions{Recursive: true, Prefix: prefix}) {
		if o.Err != nil {
			return o.Err
		}
		body, e := client.GetObject(ctx, m.Bucket, o.Key, minio.GetObjectOptions{})
		if e != nil {
			return e
		}
		h := sha256.New()
		var temp *os.File
		var dest io.Writer = h
		if os.Args[1] == "snapshot" {
			temp, e = os.CreateTemp(dir, "blob-")
			if e != nil {
				body.Close()
				return e
			}
			dest = io.MultiWriter(h, temp)
		}
		size, e := io.Copy(dest, body)
		body.Close()
		if temp != nil {
			if closeErr := temp.Close(); e == nil {
				e = closeErr
			}
		}
		if e != nil || size != o.Size {
			return errors.New("object length/readback mismatch")
		}
		sum := hex.EncodeToString(h.Sum(nil))
		items = append(items, blob{o.Key, size, sum})
		if temp != nil {
			if e = os.Rename(temp.Name(), filepath.Join(dir, sum+".blob")); e != nil {
				return e
			}
		}
	}
	b, e := json.MarshalIndent(items, "", "  ")
	if e != nil {
		return e
	}
	if os.Args[1] == "snapshot" {
		return os.WriteFile(filepath.Join(dir, "objects.json"), b, 0600)
	}
	_, e = os.Stdout.Write(b)
	return e
}
