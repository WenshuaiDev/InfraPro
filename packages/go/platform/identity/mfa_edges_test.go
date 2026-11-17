package identity

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMFAEnrollmentExpiryAndAtomicReplacement(t *testing.T) {
	x := setup(t)
	s, ctx := x.service, context.Background()
	bootstrap(t, s)
	p, original := login(t, s, "Alice", testPassword)
	start := func(mode, version string) EnrollmentSetup {
		t.Helper()
		v, e := s.StartSelfEnrollment(ctx, p, mode, version, Meta{})
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	assertState := func(state, version string, codes int) {
		t.Helper()
		v, e := s.MFA(ctx, p)
		if e != nil || v.State != state || v.FactorVersion != version || v.RecoveryCodesRemaining != codes {
			t.Fatal("unfinished enrollment changed committed factor state", e)
		}
	}
	first := start("enable", "1")
	assertState("disabled", "1", 0)
	if e := s.CancelSelfEnrollment(ctx, p, first.ID); e != nil {
		t.Fatal(e)
	}
	if _, e := s.ConfirmEnrollment(ctx, &p, "", first.ID, mfaCode(t, first.Secret, time.Now()), Meta{}); !errors.Is(e, ErrChallenge) {
		t.Fatal("cancelled enrollment completed", e)
	}
	expiring := start("enable", "1")
	s.now = func() time.Time { return time.Now().Add(11 * time.Minute) }
	if _, e := s.ConfirmEnrollment(ctx, &p, "", expiring.ID, mfaCode(t, expiring.Secret, time.Now()), Meta{}); !errors.Is(e, ErrChallenge) {
		t.Fatal("expired enrollment completed", e)
	}
	s.now = time.Now
	assertState("disabled", "1", 0)
	if _, e := s.Authenticate(ctx, original); e != nil {
		t.Fatal("unfinished setup revoked the existing session", e)
	}
	var bound AuthDelivery
	var old EnrollmentSetup
	p, bound, old = enrollMFA(t, s, p)
	replacement := start("replace", "2")
	if _, e := s.ConfirmEnrollment(ctx, &p, "", replacement.ID, "invalid", Meta{}); !errors.Is(e, ErrMFACode) {
		t.Fatal("invalid new factor accepted", e)
	}
	assertState("enabled", "2", 10)
	challenge, e := s.BeginLogin(ctx, "Alice", testPassword, "", Meta{})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.CompleteLogin(ctx, challenge.ChallengeToken, challenge.Result.ID, "totp", mfaCode(t, old.Secret, time.Now()), Meta{}); e != nil {
		t.Fatal("old factor stopped working before replacement committed", e)
	}
	if e = s.CancelSelfEnrollment(ctx, p, replacement.ID); e != nil {
		t.Fatal(e)
	}
	assertState("enabled", "2", 10)
	replacement = start("replace", "2")
	current, e := s.ConfirmEnrollment(ctx, &p, "", replacement.ID, mfaCode(t, replacement.Secret, time.Now().Add(-30*time.Second)), Meta{})
	if e != nil {
		t.Fatal(e)
	}
	// Treat the original confirmation as a lost response. It cannot be replayed
	// into a second session or used to read the one-time recovery-code response.
	replayed, e := s.ConfirmEnrollment(ctx, &p, "", replacement.ID, mfaCode(t, replacement.Secret, time.Now()), Meta{})
	if e == nil || replayed.SessionToken != "" || len(replayed.Result.RecoveryCodes) != 0 {
		t.Fatal("completed setup replay disclosed a secret or session")
	}
	if _, e = s.Authenticate(ctx, bound.SessionToken); !errors.Is(e, ErrUnauthorized) {
		t.Fatal("replacement retained the old session", e)
	}
	challenge, e = s.BeginLogin(ctx, "Alice", testPassword, "", Meta{})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.CompleteLogin(ctx, challenge.ChallengeToken, challenge.Result.ID, "recovery_code", bound.Result.RecoveryCodes[0], Meta{}); !errors.Is(e, ErrMFACode) {
		t.Fatal("replacement retained old recovery codes", e)
	}
	if _, e = s.CompleteLogin(ctx, challenge.ChallengeToken, challenge.Result.ID, "recovery_code", current.Result.RecoveryCodes[0], Meta{}); e != nil {
		t.Fatal("new configuration could not recover a lost confirmation", e)
	}
}
