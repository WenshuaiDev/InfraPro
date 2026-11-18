package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/blobstore"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/testsupport"
	httptransport "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func browserAvatars(t *testing.T, ctx context.Context, env *testsupport.Environment) *blobstore.Store {
	t.Helper()
	if os.Getenv("INFRAPRO_AVATAR_E2E") != "1" {
		return nil
	}
	bucket := "avatars-test-" + uuid.NewString()
	client, e := minio.New("127.0.0.1:58333", &minio.Options{Creds: credentials.NewStaticV4("contract-admin", "contract-admin-test-only", ""), BucketLookup: minio.BucketLookupPath})
	if e != nil {
		t.Fatal(e)
	}
	if e = client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		for o := range client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Recursive: true}) {
			if o.Err == nil {
				_ = client.RemoveObject(ctx, bucket, o.Key, minio.RemoveObjectOptions{})
			}
		}
		_ = client.RemoveBucket(ctx, bucket)
	})
	store, e := blobstore.Open("http://127.0.0.1:58333", "contract-admin", "contract-admin-test-only", bucket, "account-avatars/v1")
	if e != nil {
		t.Fatal(e)
	}
	env.Service.WithAvatarStorage(store)
	return store
}

func TestAvatarHTTPRealStorageAndScopedPermissions(t *testing.T) {
	if os.Getenv("INFRAPRO_TEST_DATABASE_URL") == "" {
		t.Skip("run scripts/test-avatars.mjs")
	}
	t.Setenv("INFRAPRO_AVATAR_E2E", "1")
	ctx := context.Background()
	env, e := testsupport.New(ctx, logger)
	if e != nil {
		t.Fatal(e)
	}
	defer env.Close()
	owner, e := env.Service.Bootstrap(ctx, "AvatarOwner", "头像测试", password, identity.Meta{})
	if e != nil {
		t.Fatal(e)
	}
	store := browserAvatars(t, ctx, env)
	server := httptest.NewServer(httptransport.NewRouter(logger, env.Service, env.Config))
	defer server.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	csrf := ""
	call := func(method, path string, body []byte, want int) []byte {
		t.Helper()
		r, _ := http.NewRequest(method, server.URL+"/api/v1"+path, bytes.NewReader(body))
		r.Header.Set("Origin", env.Config.TrustedOrigins[0])
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", csrf)
		r.Header.Set("X-Command-ID", uuid.NewString())
		response, e := client.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		data, _ := io.ReadAll(response.Body)
		if response.StatusCode != want {
			t.Fatalf("%s %s: %d %s", method, path, response.StatusCode, data)
		}
		if method == "GET" && strings.Contains(path, "/avatar") && want == 200 {
			if response.Header.Get("Cache-Control") != "private, no-store" || response.Header.Get("Content-Type") != "image/png" {
				t.Fatal("unsafe image headers")
			}
		}
		return data
	}
	jsonCall := func(method, path string, input any, want int) map[string]any {
		t.Helper()
		b, _ := json.Marshal(input)
		raw := call(method, path, b, want)
		v := map[string]any{}
		if len(raw) > 0 {
			if e := json.Unmarshal(raw, &v); e != nil {
				t.Fatal(e)
			}
		}
		return v
	}
	call("GET", "/account/avatar", nil, 401)
	csrf = jsonCall("GET", "/auth/csrf", nil, 200)["token"].(string)
	jsonCall("POST", "/auth/login", map[string]string{"username": "AvatarOwner", "password": password}, 200)
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 320, 200)))
	validCSRF := csrf
	csrf = ""
	call("PUT", "/account/avatar?expected_revision=0", b.Bytes(), 403)
	csrf = validCSRF
	call("PUT", "/account/avatar", b.Bytes(), 400)
	call("PUT", "/account/avatar?expected_revision=0", bytes.Repeat([]byte{1}, identity.AvatarMaxBytes+1), 400)
	saved := call("PUT", "/account/avatar?expected_revision=0", b.Bytes(), 200)
	if !bytes.Contains(saved, []byte(`"avatar_revision":"1"`)) {
		t.Fatal(string(saved))
	}
	self := call("GET", "/account/avatar?version=1", nil, 200)
	var objectKey string
	if e = env.Pool.QueryRow(ctx, `SELECT object_key FROM account_avatar_objects o JOIN accounts a ON a.avatar_object_id=o.id WHERE a.id=$1`, owner.ID).Scan(&objectKey); e != nil {
		t.Fatal(e)
	}
	if e = store.Delete(ctx, objectKey); e != nil {
		t.Fatal(e)
	}
	call("GET", "/account/avatar?version=1", nil, 503)
	if e = store.Put(ctx, objectKey, bytes.NewReader(self), int64(len(self)), "image/png"); e != nil {
		t.Fatal(e)
	}
	call("GET", "/account/avatar?version=1", nil, 200)
	cfg, _, e := image.DecodeConfig(bytes.NewReader(self))
	if e != nil || cfg.Width != 256 || cfg.Height != 256 {
		t.Fatal("not normalized")
	}
	call("GET", "/access/accounts/"+owner.ID+"/avatar?version=1", nil, 200)
	tenant := jsonCall("POST", "/platform/tenants", map[string]string{"name": "头像租户", "code": "avatar-scope", "administrator_account_id": owner.ID, "client_request_id": uuid.NewString(), "reason": "头像范围测试"}, 201)["tenant_id"].(string)
	members := jsonCall("GET", "/tenants/"+tenant+"/members", nil, 200)["items"].([]any)
	member := members[0].(map[string]any)
	tm := member["id"].(string)
	if member["account"].(map[string]any)["has_avatar"] != true {
		t.Fatal("tenant metadata missing")
	}
	tenantImage := "/tenants/" + tenant + "/members/" + tm + "/avatar?version=1"
	call("GET", tenantImage, nil, 200)
	project := jsonCall("POST", "/tenants/"+tenant+"/projects", map[string]string{"name": "头像项目", "code": "avatar-project", "purpose": "合成测试", "owner_tenant_membership_id": tm, "administrator_tenant_membership_id": tm, "client_request_id": uuid.NewString(), "reason": "头像范围测试"}, 200)["project_id"].(string)
	pm := jsonCall("GET", "/tenants/"+tenant+"/projects/"+project+"/members", nil, 200)["items"].([]any)[0].(map[string]any)
	if pm["avatar_revision"] != "1" {
		t.Fatal("project metadata missing")
	}
	projectImage := "/tenants/" + tenant + "/projects/" + project + "/members/" + pm["id"].(string) + "/avatar?version=1"
	call("GET", projectImage, nil, 200)
	call("GET", "/tenants/"+tenant+"/members/"+uuid.NewString()+"/avatar", nil, 404)
	call("GET", "/tenants/"+tenant+"/projects/"+project+"/members/"+tm+"/avatar", nil, 404)
	link := jsonCall("POST", "/invitations", map[string]string{"note": "头像隔离"}, 200)["url"].(string)
	token := strings.Split(link, "#token=")[1]
	jsonCall("POST", "/auth/logout", nil, 204)
	jsonCall("POST", "/public/invitations/accept", map[string]string{"token": token, "username": "AvatarOther", "display_name": "另一账号", "password": password}, 204)
	jsonCall("POST", "/auth/logout", nil, 204)
	jsonCall("POST", "/auth/login", map[string]string{"username": "AvatarOther", "password": password}, 200)
	call("GET", "/access/accounts/"+owner.ID+"/avatar", nil, 403)
	call("GET", tenantImage, nil, 404)
	call("GET", projectImage, nil, 404)
	call("PUT", "/access/accounts/"+owner.ID+"/avatar", b.Bytes(), 405)
	jsonCall("POST", "/auth/logout", nil, 204)
	jsonCall("POST", "/auth/login", map[string]string{"username": "AvatarOwner", "password": password}, 200)
	call("DELETE", "/account/avatar?expected_revision=0", nil, 409)
	call("DELETE", "/account/avatar?expected_revision=1", nil, 200)
	call("GET", tenantImage, nil, 404)
	call("GET", projectImage, nil, 404)
}
