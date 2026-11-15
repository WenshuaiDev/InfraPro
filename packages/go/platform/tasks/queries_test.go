package tasks

import (
	"strings"
	"testing"
	"time"
)

func TestTaskCursorIntegrityAndInputBounds(t *testing.T) {
	c := cursor{Account: "account-A", Filter: "mine", Authorization: "revision-A", Epoch: "restore-A", Upper: time.Now().UTC(), ID: "item-A"}
	token := sign(c, "test-only-secret")
	got, e := decode(token, "test-only-secret")
	if e != nil || got.ID != c.ID {
		t.Fatal(e)
	}
	for _, v := range []struct{ token, key string }{{token, "other-secret"}, {token + "x", "test-only-secret"}, {strings.Split(token, ".")[0] + ".eA", "test-only-secret"}, {"bad", "test-only-secret"}} {
		if _, e = decode(v.token, v.key); e == nil {
			t.Fatal("forged cursor accepted")
		}
	}
	for _, f := range []Filter{{Limit: 101}, {Limit: -1}, {Q: strings.Repeat("字", 101)}, {Source: "monitoring,arbitrary"}, {Mode: "all_accounts"}, {From: "2026-11-13T12:00:00Z", To: "2026-11-13T11:00:00Z"}, {Project: "00000000-0000-0000-0000-000000000001"}} {
		if e = f.validate(DefaultOptions()); e == nil {
			t.Fatal("unbounded or ambiguous query accepted", f)
		}
	}
}

func TestTaskSortAndTypeSetValidation(t *testing.T) {
	for _, value := range []Filter{{Sort: "created_at;drop"}, {Type: "monitoring.export,"}, {Type: "monitoring.export,arbitrary"}} {
		if value.validate(DefaultOptions()) == nil {
			t.Fatal("invalid sort or task set accepted", value)
		}
	}
	for _, sort := range []string{"", "newest", "oldest"} {
		value := Filter{Sort: sort, Type: "monitoring.query,monitoring.export,monitoring.query"}
		if err := value.validate(DefaultOptions()); err != nil || value.Type != "monitoring.export,monitoring.query" {
			t.Fatalf("type set was not canonicalized: %+v %v", value, err)
		}
	}
}
