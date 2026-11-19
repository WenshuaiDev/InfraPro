package monitoring

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/requestlimit"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestImmutableSummaryUsesSingleObjectRead(t *testing.T) {
	root := []byte(`{"Version":1,"ContentRevision":"42"}`)
	var compressed bytes.Buffer
	archive := gzip.NewWriter(&compressed)
	if _, err := archive.Write(root); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(root)
	goodDigest := hex.EncodeToString(sum[:])
	var heads, gets atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			heads.Add(1)
		case http.MethodGet:
			gets.Add(1)
		default:
			t.Errorf("unexpected storage method %s", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Last-Modified", "Wed, 18 Nov 2026 00:00:00 GMT")
		w.Header().Set("ETag", `"0123456789abcdef0123456789abcdef"`)
		if strings.HasSuffix(r.URL.Path, "/missing") {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("<Error><Code>NoSuchKey</Code><Message>missing</Message></Error>"))
			return
		}
		body := root
		if strings.HasSuffix(r.URL.Path, ".gz") {
			body = compressed.Bytes()
		}
		if strings.HasSuffix(r.URL.Path, "invalid-gzip") {
			body = []byte("invalid compression header")
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		if r.Method == http.MethodGet {
			_, _ = w.Write(body)
		}
	}))
	defer server.Close()
	objects, err := minio.New(strings.TrimPrefix(server.URL, "http://"), &minio.Options{
		Region: "us-east-1", Creds: credentials.NewStaticV4("fixture", "fixture-only", ""),
		Transport: requestlimit.New(1).Transport(http.DefaultTransport),
	})
	if err != nil {
		t.Fatal(err)
	}
	storage := &Storage{objects: objects, config: StorageConfig{Bucket: "indices"}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var result summaryRoot
	for _, tc := range []struct {
		key           string
		hashed, valid bool
	}{
		{"tenant/root", false, true},
		{"tenant/" + goodDigest + ".json.gz", true, true},
		{"tenant/" + strings.Repeat("0", 64) + ".json.gz", true, false},
		{"tenant/invalid-gzip", true, false},
		{"tenant/missing", false, false},
	} {
		before := gets.Load()
		err := storage.summaryObject(ctx, tc.key, &result, tc.hashed)
		if (err == nil) != tc.valid {
			t.Fatalf("%s valid=%t: %v", tc.key, tc.valid, err)
		}
		if tc.valid && (result.Version != 1 || result.ContentRevision != "42") {
			t.Fatalf("incorrect fixed summary %+v", result)
		}
		if strings.HasSuffix(tc.key, "/missing") && minio.ToErrorResponse(err).Code != "NoSuchKey" {
			t.Fatalf("storage absence lost its original code: %v", err)
		}
		if !tc.valid && strings.HasSuffix(tc.key, ".gz") && !strings.Contains(err.Error(), "hash mismatch") {
			t.Fatalf("damaged immutable index was not verified: %v", err)
		}
		if heads.Load() != 0 || gets.Load()-before != 1 {
			t.Fatalf("complete index read used %d HEAD and %d GET", heads.Load(), gets.Load()-before)
		}
	}
	reader, err := storage.get(ctx, "tenant/root")
	if err != nil {
		t.Fatal(err)
	}
	_ = reader.Close()
	if heads.Load() != 1 {
		t.Fatal("the streaming caller lost its initial storage check")
	}
}
