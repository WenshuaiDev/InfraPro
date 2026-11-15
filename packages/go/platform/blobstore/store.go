// Package blobstore is the finite private-object adapter shared by domain stores.
package blobstore

import (
	"context"
	"fmt"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"io"
	"io/fs"
	"net/url"
	"strings"
)

type Store struct {
	client         *minio.Client
	bucket, prefix string
}

func Open(endpoint, accessKey, secretKey, bucket, prefix string) (*Store, error) {
	u, e := url.Parse(endpoint)
	if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.Path != "" && u.Path != "/" || bucket == "" || prefix == "" {
		return nil, fmt.Errorf("invalid private object storage configuration")
	}
	client, e := minio.New(u.Host, &minio.Options{Creds: credentials.NewStaticV4(accessKey, secretKey, ""), Secure: u.Scheme == "https", BucketLookup: minio.BucketLookupPath})
	if e != nil {
		return nil, e
	}
	return &Store{client, bucket, prefix}, nil
}
func (s *Store) key(key string) (string, error) {
	if !strings.HasPrefix(key, s.prefix+"/") || strings.Contains(key, "..") || strings.Contains(key, "\\") {
		return "", fmt.Errorf("object key outside domain namespace")
	}
	return key, nil
}
func (s *Store) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	if _, e := s.key(key); e != nil {
		return e
	}
	opts := minio.PutObjectOptions{ContentType: contentType, SendContentMd5: true, NumThreads: 1}
	opts.SetMatchETagExcept("*")
	_, e := s.client.PutObject(ctx, s.bucket, key, r, size, opts)
	return e
}
func (s *Store) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	if _, e := s.key(key); e != nil {
		return nil, e
	}
	o, e := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if e != nil {
		return nil, e
	}
	if _, e = o.Stat(); e != nil {
		o.Close()
		code := minio.ToErrorResponse(e).Code
		if code == "NoSuchKey" || code == "NoSuchObject" {
			return nil, fmt.Errorf("private object missing: %w", fs.ErrNotExist)
		}
		return nil, e
	}
	return o, nil
}
func (s *Store) Delete(ctx context.Context, key string) error {
	if _, e := s.key(key); e != nil {
		return e
	}
	return s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}

// Inventory is diagnostic only. Domains must never turn unregistered keys into
// deletion candidates. Results stay inside the adapter's private namespace.
func (s *Store) Inventory(ctx context.Context, after string, limit int) ([]string, string, error) {
	if limit < 1 || limit > 1000 {
		return nil, "", fmt.Errorf("invalid inventory limit")
	}
	if after != "" {
		if _, e := s.key(after); e != nil {
			return nil, "", e
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	out := []string{}
	for object := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{Prefix: s.prefix + "/", Recursive: true, StartAfter: after}) {
		if object.Err != nil {
			return nil, "", object.Err
		}
		if len(out) == limit {
			return out, out[len(out)-1], nil
		}
		out = append(out, object.Key)
	}
	return out, "", nil
}

// Probe performs a real bounded namespace read, without writing or exposing keys.
func (s *Store) Probe(ctx context.Context) error {
	_, _, err := s.Inventory(ctx, "", 1)
	return err
}
