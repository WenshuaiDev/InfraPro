package identity

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"sync"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
)

type avatarTestStore struct {
	mu       sync.Mutex
	objects  map[string][]byte
	failPut  bool
	afterGet func()
}

func (s *avatarTestStore) Put(_ context.Context, key string, r io.Reader, _ int64, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failPut {
		return errors.New("offline")
	}
	v, e := io.ReadAll(r)
	s.objects[key] = v
	return e
}
func (s *avatarTestStore) Get(_ context.Context, key string) (io.ReadCloser, error) {
	s.mu.Lock()
	v, ok := s.objects[key]
	hook := s.afterGet
	s.afterGet = nil
	s.mu.Unlock()
	if hook != nil {
		hook()
	}
	if !ok {
		return nil, errors.New("missing")
	}
	return io.NopCloser(bytes.NewReader(v)), nil
}
func (s *avatarTestStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objects, key)
	return nil
}
func avatarPNG(t *testing.T, jpg bool) []byte {
	t.Helper()
	im := image.NewNRGBA(image.Rect(0, 0, 640, 360))
	for y := 0; y < 360; y++ {
		for x := 0; x < 640; x++ {
			im.Set(x, y, color.NRGBA{uint8(x % 256), uint8(y % 256), 150, 255})
		}
	}
	var b bytes.Buffer
	var e error
	if jpg {
		e = jpeg.Encode(&b, im, nil)
	} else {
		e = png.Encode(&b, im)
	}
	if e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}

func TestAvatarLifecycleAuthorizationAndRecovery(t *testing.T) {
	env := setup(t)
	s := env.service
	ctx := context.Background()
	bootstrap(t, s)
	owner, _ := login(t, s, "Alice", testPassword)
	store := &avatarTestStore{objects: map[string][]byte{}}
	s.WithAvatarStorage(store)
	check := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	empty, e := s.Me(ctx, owner)
	check(e)
	if empty.HasAvatar || empty.AvatarRevision != "0" {
		t.Fatal("legacy default")
	}
	first, e := s.UpdateAvatar(ctx, owner, "0", avatarPNG(t, false), Meta{})
	check(e)
	if !first.HasAvatar || first.AvatarRevision != "1" {
		t.Fatal(first)
	}
	data, e := s.ReadAvatar(ctx, owner, AvatarScope{Kind: "self"}, "1")
	check(e)
	cfg, format, e := image.DecodeConfig(bytes.NewReader(data))
	check(e)
	if format != "png" || cfg.Width != 256 || cfg.Height != 256 {
		t.Fatal("noncanonical output")
	}
	_, e = s.UpdateAvatar(ctx, owner, "0", avatarPNG(t, true), Meta{})
	requireCode(t, e, "avatar_changed")
	_, e = s.UpdateAvatar(ctx, owner, "1", []byte("<svg></svg>"), Meta{})
	requireCode(t, e, "validation_failed")
	store.failPut = true
	_, e = s.UpdateAvatar(ctx, owner, "1", avatarPNG(t, true), Meta{})
	requireCode(t, e, "avatar_storage_unavailable")
	store.failPut = false
	preserved, e := s.Me(ctx, owner)
	check(e)
	if preserved.AvatarRevision != "1" {
		t.Fatal("failed write replaced avatar")
	}
	second, e := s.UpdateAvatar(ctx, owner, "1", avatarPNG(t, true), Meta{})
	check(e)
	if second.AvatarRevision != "2" {
		t.Fatal(second)
	}
	_, e = s.ReadAvatar(ctx, owner, AvatarScope{Kind: "self"}, "1")
	requireCode(t, e, "not_found")
	// Simulate object loss and recovery from a byte-for-byte backup.
	store.mu.Lock()
	backup := map[string][]byte{}
	for k, v := range store.objects {
		backup[k] = v
	}
	store.objects = map[string][]byte{}
	for key, value := range backup {
		corrupted := bytes.Clone(value)
		corrupted[0] ^= 0xff
		store.objects[key] = corrupted
	}
	store.mu.Unlock()
	_, e = s.ReadAvatar(ctx, owner, AvatarScope{Kind: "self"}, "2")
	requireCode(t, e, "unavailable")
	store.mu.Lock()
	store.objects = map[string][]byte{}
	store.mu.Unlock()
	_, e = s.ReadAvatar(ctx, owner, AvatarScope{Kind: "self"}, "2")
	requireCode(t, e, "unavailable")
	store.mu.Lock()
	store.objects = backup
	store.mu.Unlock()
	_, e = s.ReadAvatar(ctx, owner, AvatarScope{Kind: "self"}, "2")
	check(e)
	invite(t, s, owner, "Other")
	other, _ := login(t, s, "Other", testPassword)
	_, e = s.ReadAvatar(ctx, other, AvatarScope{Kind: "platform", ID: owner.Account.ID}, "2")
	requireCode(t, e, "permission_denied")
	_, e = s.ReadAvatar(ctx, other, AvatarScope{Kind: "self"}, "2")
	requireCode(t, e, "not_found")
	// Permission is checked again after storage returns, even on a previously
	// authorized request; releasing a session while I/O runs cannot leak bytes.
	store.afterGet = func() { check(s.Revoke(ctx, owner, owner.Session.ID, Meta{})) }
	_, e = s.ReadAvatar(ctx, owner, AvatarScope{Kind: "self"}, "2")
	requireCode(t, e, "unauthenticated")
	owner, _ = login(t, s, "Alice", testPassword)
	removed, e := s.RemoveAvatar(ctx, owner, "2", Meta{})
	check(e)
	if removed.HasAvatar || removed.AvatarRevision != "3" {
		t.Fatal(removed)
	}
	_, e = s.ReadAvatar(ctx, owner, AvatarScope{Kind: "self"}, "")
	requireCode(t, e, "not_found")
	_, e = env.pool.Exec(ctx, `UPDATE account_avatar_objects SET cleanup_after=now()-interval '1 second' WHERE cleanup_after IS NOT NULL`)
	check(e)
	check(s.SweepAvatars(ctx))
	check(s.SweepAvatars(ctx))
	if len(store.objects) != 0 {
		t.Fatal("cleanup leaked objects")
	}
	var events int
	check(env.pool.QueryRow(ctx, `SELECT count(*) FROM account_security_events WHERE event_type IN('avatar_updated','avatar_removed')`).Scan(&events))
	if events != 3 {
		t.Fatalf("audit events=%d", events)
	}
	_, e = env.pool.Exec(ctx, `DELETE FROM account_roles WHERE account_id=$1`, other.Account.ID)
	check(e)
	_, e = s.UpdateAvatar(ctx, other, "0", avatarPNG(t, false), Meta{})
	if !errors.Is(e, access.ErrDenied) {
		requireCode(t, e, "permission_denied")
	}
}

func TestAvatarConcurrentReplacement(t *testing.T) {
	env := setup(t)
	s := env.service
	ctx := context.Background()
	bootstrap(t, s)
	p, _ := login(t, s, "Alice", testPassword)
	store := &avatarTestStore{objects: map[string][]byte{}}
	s.WithAvatarStorage(store)
	input := avatarPNG(t, false)
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() { _, e := s.UpdateAvatar(ctx, p, "0", input, Meta{}); results <- e })
	}
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for e := range results {
		if e == nil {
			success++
		} else if PublicError(e).Code == "avatar_changed" {
			conflict++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatal(success, conflict)
	}
	if _, e := env.pool.Exec(ctx, `UPDATE account_avatar_objects SET cleanup_after=now()-interval '1 second' WHERE state='pending'`); e != nil {
		t.Fatal(e)
	}
	if e := s.SweepAvatars(ctx); e != nil {
		t.Fatal(e)
	}
	if len(store.objects) != 1 {
		t.Fatal("current object reclaimed or orphan leaked")
	}
}
