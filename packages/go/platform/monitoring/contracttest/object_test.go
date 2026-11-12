//go:build monitoringintegration

package contracttest

import (
	"bytes"
	"crypto/md5"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestObjectContract(t *testing.T) {
	admin := s3Client(t, "contract-admin", "contract-admin-test-only")
	waitS3(t, admin)
	for _, bucket := range []string{"contract-source", "contract-ch"} {
		exists, err := admin.BucketExists(t.Context(), bucket)
		must(t, err)
		if !exists {
			must(t, admin.MakeBucket(t.Context(), bucket, minio.MakeBucketOptions{}))
		}
	}
	c := s3Client(t, "contract-source", "contract-source-test-only")
	core := minio.Core{Client: c}
	key := "sources/" + uuid.NewString()
	data := make([]byte, 5*1024*1024+137)
	rng := rand.New(rand.NewPCG(42, 19))
	for i := range data {
		data[i] = byte(rng.Uint32())
	}
	expected := sha256.Sum256(data)
	upload, err := core.NewMultipartUpload(t.Context(), "contract-source", key, minio.PutObjectOptions{UserMetadata: map[string]string{"sha256": fmt.Sprintf("%x", expected)}})
	must(t, err)
	parts := []minio.CompletePart{}
	for i, p := range [][]byte{data[:5*1024*1024], data[5*1024*1024:]} {
		sum := md5.Sum(p)
		part, err := core.PutObjectPart(t.Context(), "contract-source", key, upload, i+1, bytes.NewReader(p), int64(len(p)), minio.PutObjectPartOptions{Md5Base64: base64.StdEncoding.EncodeToString(sum[:])})
		must(t, err)
		parts = append(parts, minio.CompletePart{PartNumber: i + 1, ETag: part.ETag})
	}
	t.Run("incomplete_upload_invisible", func(t *testing.T) {
		_, err := c.StatObject(t.Context(), "contract-source", key, minio.StatObjectOptions{})
		if minio.ToErrorResponse(err).Code != "NoSuchKey" {
			t.Fatal("incomplete object was visible or unclassified", err)
		}
	})
	info, err := core.CompleteMultipartUpload(t.Context(), "contract-source", key, upload, parts, minio.PutObjectOptions{})
	must(t, err)
	t.Run("complete_sha256_and_exact_ranges", func(t *testing.T) {
		if info.ETag == fmt.Sprintf("%x", expected) {
			t.Fatal("fixture cannot distinguish ETag and SHA256")
		}
		obj, err := c.GetObject(t.Context(), "contract-source", key, minio.GetObjectOptions{})
		must(t, err)
		defer obj.Close()
		got, err := io.ReadAll(obj)
		must(t, err)
		if sha256.Sum256(got) != expected {
			t.Fatal("SHA256 changed")
		}
		for _, r := range [][2]int64{{0, 0}, {5*1024*1024 - 10, 5*1024*1024 + 10}, {int64(len(data) - 1), int64(len(data) - 1)}} {
			opts := minio.GetObjectOptions{}
			must(t, opts.SetRange(r[0], r[1]))
			o, e := c.GetObject(t.Context(), "contract-source", key, opts)
			must(t, e)
			got, e := io.ReadAll(o)
			_ = o.Close()
			must(t, e)
			if !bytes.Equal(got, data[r[0]:r[1]+1]) {
				t.Fatal("range mismatch", r)
			}
		}
	})
	t.Run("abort_and_resume_parts", func(t *testing.T) {
		k := "sources/" + uuid.NewString()
		id, e := core.NewMultipartUpload(t.Context(), "contract-source", k, minio.PutObjectOptions{})
		must(t, e)
		_, e = core.PutObjectPart(t.Context(), "contract-source", k, id, 1, bytes.NewReader(data), int64(len(data)), minio.PutObjectPartOptions{})
		must(t, e)
		listed, e := core.ListObjectParts(t.Context(), "contract-source", k, id, 0, 100)
		must(t, e)
		if len(listed.ObjectParts) != 1 {
			t.Fatal("resume lost part")
		}
		must(t, core.AbortMultipartUpload(t.Context(), "contract-source", k, id))
		_, e = core.ListObjectParts(t.Context(), "contract-source", k, id, 0, 100)
		if minio.ToErrorResponse(e).Code != "NoSuchUpload" {
			t.Fatal("abort did not remove upload", e)
		}
	})
	t.Run("checksum_mismatch_rejected", func(t *testing.T) {
		k := "sources/" + uuid.NewString()
		id, e := core.NewMultipartUpload(t.Context(), "contract-source", k, minio.PutObjectOptions{})
		must(t, e)
		defer core.AbortMultipartUpload(t.Context(), "contract-source", k, id)
		_, e = core.PutObjectPart(t.Context(), "contract-source", k, id, 1, bytes.NewReader(data), int64(len(data)), minio.PutObjectPartOptions{Md5Base64: base64.StdEncoding.EncodeToString(make([]byte, 16))})
		if e == nil {
			t.Fatal("provider accepted corrupt part checksum")
		}
	})
	t.Run("independent_namespaces", func(t *testing.T) {
		_, e := c.PutObject(t.Context(), "contract-ch", "forbidden", strings.NewReader("x"), 1, minio.PutObjectOptions{})
		if minio.ToErrorResponse(e).Code != "AccessDenied" {
			t.Fatal("source identity can mutate database namespace", e)
		}
	})
	t.Run("sealed_object_conditional_write", func(t *testing.T) {
		opts := minio.PutObjectOptions{}
		opts.SetMatchETagExcept("*")
		_, e := c.PutObject(t.Context(), "contract-source", key, strings.NewReader("overwrite"), 9, opts)
		if minio.ToErrorResponse(e).Code != "PreconditionFailed" {
			t.Fatal("immutable source overwrite accepted", e)
		}
	})
	t.Run("tls_verified_and_transient_retry", func(t *testing.T) {
		target, _ := url.Parse("http://127.0.0.1:58333")
		proxy := httputil.NewSingleHostReverseProxy(target)
		var calls atomic.Int32
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if calls.Add(1) == 1 {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(503)
				_, _ = io.WriteString(w, `<Error><Code>SlowDown</Code><Message>contract transient fault</Message></Error>`)
				return
			}
			proxy.ServeHTTP(w, r)
		}))
		defer server.Close()
		pool := x509.NewCertPool()
		pool.AddCert(server.Certificate())
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool}
		defer transport.CloseIdleConnections()
		secured, e := minio.New(strings.TrimPrefix(server.URL, "https://"), &minio.Options{Creds: credentials.NewStaticV4("contract-source", "contract-source-test-only", ""), Secure: true, Region: "us-east-1", Transport: transport})
		must(t, e)
		o, e := secured.GetObject(t.Context(), "contract-source", key, minio.GetObjectOptions{})
		must(t, e)
		defer o.Close()
		got, e := io.ReadAll(o)
		must(t, e)
		if sha256.Sum256(got) != expected || calls.Load() < 2 {
			t.Fatal("TLS/retry contract failed")
		}
		untrusted := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}}
		defer untrusted.CloseIdleConnections()
		response, e := untrusted.Get(server.URL)
		if e == nil {
			response.Body.Close()
			t.Fatal("untrusted TLS accepted")
		}
	})
	must(t, c.RemoveObject(t.Context(), "contract-source", key, minio.RemoveObjectOptions{}))
}
