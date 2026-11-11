package identity

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestIntegrationInvitationFiltersCountsAndOwnerDetails(t *testing.T) {
	x := setup(t)
	s := x.service
	ctx := context.Background()
	bootstrap(t, s)
	p, _ := login(t, s, "Alice", testPassword)
	invite(t, s, p, "Bob") // One used invitation owned by Alice.
	bob, _ := login(t, s, "Bob", testPassword)
	_, e := x.pool.Exec(ctx, `INSERT INTO account_invitations(inviter_id,token_hash,note,created_at,expires_at,revoked_at)
		SELECT $1,'pagination-'||n,'历史邀请 '||n,now()-interval '2 days',now()-interval '1 day',now() FROM generate_series(1,51) n`, p.Account.ID)
	if e != nil {
		t.Fatal(e)
	}
	var pendingID, foreignID string
	e = x.pool.QueryRow(ctx, `INSERT INTO account_invitations(inviter_id,token_hash,note,expires_at) VALUES($1,'pending-note','Bridge 100%_汉字🙂',now()+interval '1 hour') RETURNING id::text`, p.Account.ID).Scan(&pendingID)
	if e != nil {
		t.Fatal(e)
	}
	_, e = x.pool.Exec(ctx, `INSERT INTO account_invitations(inviter_id,token_hash,note,expires_at) VALUES($1,'expired-note','已经过期',now()-interval '1 second')`, p.Account.ID)
	if e != nil {
		t.Fatal(e)
	}
	e = x.pool.QueryRow(ctx, `INSERT INTO account_invitations(inviter_id,token_hash,note,expires_at) VALUES($1,'foreign-note','其他账号',now()+interval '1 day') RETURNING id::text`, bob.Account.ID).Scan(&foreignID)
	if e != nil {
		t.Fatal(e)
	}
	all, e := s.Invitations(ctx, p, InvitationFilter{})
	if e != nil {
		t.Fatal(e)
	}
	want := InvitationCounts{All: 54, Pending: 1, Used: 1, Expired: 1, Revoked: 51}
	if all.Counts != want || all.Total != 54 || len(all.Items) != 50 || all.NextCursor == "" {
		t.Fatalf("wrong first page totals: %+v, %d, %d", all.Counts, all.Total, len(all.Items))
	}
	seen := map[string]bool{}
	for _, item := range all.Items {
		seen[item.ID] = true
	}
	next, e := s.Invitations(ctx, p, InvitationFilter{Cursor: all.NextCursor})
	if e != nil || len(next.Items) != 4 || next.NextCursor != "" || next.Counts != want || next.Total != 54 {
		t.Fatal("incorrect second page", e, len(next.Items), next.Counts)
	}
	for _, item := range next.Items {
		if seen[item.ID] {
			t.Fatal("duplicate item across tied-timestamp pages")
		}
		seen[item.ID] = true
	}
	for _, query := range []string{" bridge ", "%_", "汉字🙂"} {
		page, err := s.Invitations(ctx, p, InvitationFilter{Query: query, Status: "pending"})
		if err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != pendingID || page.Counts != want {
			t.Fatal("search or unfiltered counts incorrect", query, err, page.Counts)
		}
	}
	page, e := s.Invitations(ctx, p, InvitationFilter{Query: "does-not-exist"})
	if e != nil || len(page.Items) != 0 || page.Total != 0 || page.Counts != want {
		t.Fatal("empty matches lost global counts", e)
	}
	for _, state := range []string{"used", "expired", "pending", "revoked"} {
		page, e = s.Invitations(ctx, p, InvitationFilter{Status: state})
		if e != nil {
			t.Fatal(e)
		}
		for _, item := range page.Items {
			if item.Status != state {
				t.Fatal("state filter leaked a different status")
			}
		}
		if state == "revoked" && (page.Total != 51 || len(page.Items) != 50 || page.NextCursor == "") {
			t.Fatal("filtered pagination lost old matches")
		}
	}
	for _, filter := range []InvitationFilter{{Status: "unknown"}, {Query: strings.Repeat("🙂", 201)}, {Cursor: "invalid"}, {Cursor: all.NextCursor, Query: "Bridge"}, {Cursor: all.NextCursor, Status: "revoked"}} {
		_, e = s.Invitations(ctx, p, filter)
		requireCode(t, e, "validation_failed")
	}
	item, e := s.Invitation(ctx, p, pendingID)
	if e != nil || item.Status != "pending" || item.Note != "Bridge 100%_汉字🙂" {
		t.Fatal("detail mismatch", e)
	}
	encoded, e := json.Marshal(item)
	if e != nil || strings.Contains(string(encoded), "token") || strings.Contains(string(encoded), "url") {
		t.Fatal("detail exposed credential material", e)
	}
	for _, id := range []string{foreignID, "00000000-0000-0000-0000-000000000000", "invalid"} {
		_, e = s.Invitation(ctx, p, id)
		requireCode(t, e, "not_found")
	}
	_, e = s.Invitation(ctx, bob, pendingID)
	requireCode(t, e, "permission_denied")
	_, e = s.Invitations(ctx, bob, InvitationFilter{})
	requireCode(t, e, "permission_denied")
	if e = s.RevokeInvitation(ctx, p, pendingID, Meta{}); e != nil {
		t.Fatal(e)
	}
	item, e = s.Invitation(ctx, p, pendingID)
	if e != nil || item.Status != "revoked" {
		t.Fatal("detail did not update after revoke", e)
	}
	page, e = s.Invitations(ctx, p, InvitationFilter{Status: "pending"})
	if e != nil || page.Total != 0 || page.Counts.Pending != 0 || page.Counts.Revoked != 52 {
		t.Fatal("counts did not update after revoke", e, page.Counts)
	}
}
