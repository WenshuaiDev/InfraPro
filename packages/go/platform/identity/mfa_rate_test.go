package identity

import (
	"context"
	"log/slog"
	"testing"

	"github.com/redis/go-redis/v9"
)

func TestMFAAccountRequestBudgetAcrossIPs(t *testing.T) {
	x := setup(t)
	s := x.service
	ctx := context.Background()
	bootstrap(t, s)
	p, _ := login(t, s, "Alice", testPassword)
	p, bound, _ := enrollMFA(t, s, p)
	challenges := make([]AuthDelivery, 3)
	for i := range challenges {
		var e error
		challenges[i], e = s.BeginLogin(ctx, "Alice", testPassword, "", Meta{})
		if e != nil {
			t.Fatal(e)
		}
	}
	// Use a new namespace for the isolated minute bucket, retaining the same
	// durable account/challenges. This leaves enrollment's own request separate.
	cfg := x.cfg
	cfg.RedisKeyPrefix += "mfa-budget:"
	cfg.CredentialRequestLimit = 2
	nodes := make([]*Service, 2)
	for i := range nodes {
		var e error
		nodes[i], e = NewService(x.pool, x.redis, cfg, slog.Default())
		if e != nil {
			t.Fatal(e)
		}
	}
	for i, challenge := range challenges {
		code := "invalid"
		if i == 2 {
			code = bound.Result.RecoveryCodes[0]
		}
		_, e := nodes[i%2].CompleteLogin(ctx, challenge.ChallengeToken, challenge.Result.ID, "recovery_code", code, Meta{IP: []string{"192.0.2.1", "192.0.2.2", "192.0.2.3"}[i]})
		want := "mfa_code_invalid"
		if i == 2 {
			want = "rate_limited"
		}
		requireCode(t, e, want)
	}
	var failures, used int
	if e := x.pool.QueryRow(ctx, `SELECT failures,(SELECT count(*) FROM account_mfa_recovery_codes WHERE account_id=$2 AND used_at IS NOT NULL) FROM account_rate_limits WHERE key_hash=$1`, digest("mfa:"+p.Account.ID), p.Account.ID).Scan(&failures, &used); e != nil || failures != 2 || used != 0 {
		t.Fatal("request budget consumed a factor or changed durable guess count", e)
	}
	// A missing Redis dependency must not be mistaken for an unused bucket.
	dead := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1})
	defer dead.Close()
	failed, e := NewService(x.pool, dead, cfg, slog.Default())
	if e != nil {
		t.Fatal(e)
	}
	_, e = failed.CompleteLogin(ctx, challenges[2].ChallengeToken, challenges[2].Result.ID, "recovery_code", bound.Result.RecoveryCodes[0], Meta{})
	requireCode(t, e, "unavailable")
}
