package monitoring

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/google/uuid"
)

func TestImportListQueryAndBoundCursor(t *testing.T) {
	q, err := normalizeImportQuery(ImportQuery{Search: "  桥梁_%  ", States: "ready,failed,ready", Formats: "xlsx,csv", Limit: 10})
	if err != nil || q.Search != "桥梁_%" || q.States != "failed,ready" || q.Formats != "csv,xlsx" || q.Sort != "newest" {
		t.Fatalf("normalized query: %+v, %v", q, err)
	}
	for _, bad := range []ImportQuery{{Search: strings.Repeat("界", 201)}, {Search: "a\nb"}, {States: "published,unknown"}, {Formats: "pdf"}, {Sort: "id"}, {Limit: 11}, {Limit: -1}} {
		if _, e := normalizeImportQuery(bad); e == nil {
			t.Fatalf("accepted invalid filter %+v", bad)
		}
	}
	a := access.Actor{ID: uuid.NewString()}
	scope := TenantScope(uuid.NewString())
	key := importCursorKey(a, scope, q)
	now := time.Now().UTC()
	q.Cursor = "imports.v1." + base64.RawURLEncoding.EncodeToString(encodeJSON(importCursor{listBoundary{&now, uuid.NewString()}, key, "authorization-1"}))
	parsed, e := parseImportCursor(q, importCursorKey(a, scope, q))
	if e != nil || parsed.Authorization != "authorization-1" {
		t.Fatalf("cursor roundtrip: %+v %v", parsed, e)
	}
	for _, changed := range []ImportQuery{{Search: "other", Limit: 10}, {States: "ready", Limit: 10}, {Formats: "xlsx", Limit: 10}, {Sort: "oldest", Limit: 10}, {Limit: 20}} {
		changed.Cursor = q.Cursor
		if _, e := parseImportCursor(changed, importCursorKey(a, scope, changed)); e == nil {
			t.Fatal("cursor reused for a different query")
		}
	}
	if _, e := parseImportCursor(q, importCursorKey(access.Actor{ID: uuid.NewString()}, scope, q)); e == nil {
		t.Fatal("cursor reused by a different actor")
	}
	if _, e := parseImportCursor(q, importCursorKey(a, TenantScope(uuid.NewString()), q)); e == nil {
		t.Fatal("cursor reused across tenants")
	}
	for _, bad := range []string{"imports.v1.bad", "imports.v1." + base64.RawURLEncoding.EncodeToString([]byte(`{}`)), strings.Repeat("x", 2050)} {
		q.Cursor = bad
		if _, e := parseImportCursor(q, key); e == nil {
			t.Fatal("accepted malformed cursor")
		}
	}
}
func TestImportListSearchOnlyVisibleMetadata(t *testing.T) {
	b := ImportBatch{ID: uuid.NewString(), Name: "共享批次（当前授权范围）", State: "published", ScopeClipped: true}
	if !matchesImport(b, ImportQuery{Search: "共享批次"}) || !matchesImport(b, ImportQuery{Search: strings.ToUpper(b.ID[:8])}) {
		t.Fatal("visible name and identifier must be searchable")
	}
	if matchesImport(b, ImportQuery{Search: "secret.csv"}) || matchesImport(b, ImportQuery{Formats: "csv"}) {
		t.Fatal("partial view exposed hidden source metadata")
	}
	b = ImportBatch{Name: "literal_%_TEST.CSV", State: "failed"}
	if !matchesImport(b, ImportQuery{Search: "_%", Formats: "csv", States: "ready,failed"}) || matchesImport(b, ImportQuery{Search: "%other"}) || matchesImport(b, ImportQuery{States: "published"}) {
		t.Fatal("literal filters or multi-state matching failed")
	}
}
