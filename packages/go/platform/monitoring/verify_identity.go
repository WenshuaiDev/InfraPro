package monitoring

import (
	"cmp"
	"context"
	"fmt"
)

// External sorting bounds memory even when a channel has a long adoption
// history. Exhausting the shared temporary budget aborts verification.
type identityVerifier struct {
	spool           *spool
	active, history rowSorter
}

func newIdentityVerifier(parent string) (*identityVerifier, error) {
	s, e := newSpool(parent)
	if e != nil {
		return nil, e
	}
	v := &identityVerifier{spool: s}
	v.active = rowSorter{s: s, compare: compareRaw}
	v.history = rowSorter{s: s, compare: func(a, b rawObservation) int {
		if n := cmp.Compare(a.Observation, b.Observation); n != 0 {
			return n
		}
		return cmp.Compare(a.Adoption, b.Adoption)
	}}
	return v, nil
}
func (v *identityVerifier) close() { v.spool.close() }
func (v *identityVerifier) add(r rawObservation, active bool) error {
	if e := v.history.add(r); e != nil {
		return e
	}
	if active {
		return v.active.add(r)
	}
	return nil
}
func stableIdentityPayload(r rawObservation) string {
	// These fields describe a particular adoption, not the retained observation.
	r.Adoption, r.Batch, r.Mapping, r.First = "", "", 0, false
	return r.hash()
}
func (v *identityVerifier) verify(ctx context.Context, fail func(string, string)) error {
	path, e := v.active.finish(ctx)
	if e != nil {
		return e
	}
	it, e := openRows(path)
	if e != nil {
		return e
	}
	var prior rawObservation
	seen := false
	for it.next() {
		if e = ctx.Err(); e != nil {
			it.close()
			return e
		}
		if seen && prior.Channel == it.row.Channel && prior.At == it.row.At {
			fail(it.row.Observation, "duplicate_active_observation")
		}
		prior, seen = it.row, true
	}
	it.close()
	if it.err != nil {
		return it.err
	}
	v.spool.remove(path)
	path, e = v.history.finish(ctx)
	if e != nil {
		return e
	}
	it, e = openRows(path)
	if e != nil {
		return e
	}
	defer it.close()
	seen = false
	var first uint64
	var payload string
	finish := func() {
		if seen && first != 1 {
			fail(prior.Observation, "first_adoption_identity_mismatch")
		}
	}
	for it.next() {
		if e = ctx.Err(); e != nil {
			return e
		}
		r := it.row
		if !seen || prior.Observation != r.Observation {
			finish()
			first, payload = 0, stableIdentityPayload(r)
		} else {
			if prior.Adoption == r.Adoption {
				fail(r.Observation, "duplicate_adoption_identity")
			}
			if payload != stableIdentityPayload(r) {
				fail(r.Observation, "observation_identity_payload_mismatch")
			}
		}
		if r.First {
			first++
		}
		prior, seen = r, true
	}
	finish()
	return it.err
}
func (v *identityVerifier) requireValid(ctx context.Context) error {
	var mismatch error
	e := v.verify(ctx, func(_, code string) {
		if mismatch == nil {
			mismatch = fmt.Errorf("raw integrity check: %s", code)
		}
	})
	if e != nil {
		return e
	}
	return mismatch
}
