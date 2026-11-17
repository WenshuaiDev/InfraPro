package spatial

import (
	"context"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
)

type projectionObjects struct {
	calls  map[ObjectRef]int
	denied bool
}

func (r *projectionObjects) ObjectTx(_ context.Context, _ pgx.Tx, _ access.Actor, _ Scope, ref ObjectRef) (Object, error) {
	r.calls[ref]++
	if r.denied {
		return Object{}, k.Problem("permission_denied", "restricted", 403)
	}
	return Object{ObjectRef: ref, FacilityID: "facility", BasisHash: "current"}, nil
}
func (r *projectionObjects) Objects(context.Context, access.Actor, Scope, Filter) (List[Object], error) {
	return List[Object]{}, nil
}

func TestProjectionChecksDistinctObjectsWithinEachTransaction(t *testing.T) {
	reader := &projectionObjects{calls: map[ObjectRef]int{}}
	service := New(nil, reader)
	draft := func() Draft {
		d := Draft{}
		for i := 0; i < 600; i++ {
			d.ComponentLinks = append(d.ComponentLinks, ComponentLink{ComponentID: "lining", BasisHash: "old"})
		}
		for i := 0; i < 3; i++ {
			d.Placements = append(d.Placements, Placement{PointID: "point", BasisHash: "current"})
		}
		return d
	}
	first := draft()
	if err := service.projectDraftTx(context.Background(), nil, access.Actor{}, k.Authorization{}, &first); err != nil {
		t.Fatal(err)
	}
	if len(first.ComponentLinks) != 600 || len(first.Placements) != 3 || !first.ComponentLinks[0].BasisChanged || first.Placements[0].BasisChanged {
		t.Fatal("projection lost bindings or basis changes")
	}
	if reader.calls[ObjectRef{"component", "lining"}] != 1 || reader.calls[ObjectRef{"point", "point"}] != 1 {
		t.Fatalf("redundant object checks: %v", reader.calls)
	}
	// A later request must observe revoked access; nothing is cached on the service.
	reader.denied = true
	second := draft()
	if err := service.projectDraftTx(context.Background(), nil, access.Actor{}, k.Authorization{}, &second); err != nil {
		t.Fatal(err)
	}
	if len(second.ComponentLinks) != 0 || len(second.Placements) != 0 {
		t.Fatal("stale authorization leaked bindings")
	}
	if reader.calls[ObjectRef{"component", "lining"}] != 2 {
		t.Fatal("next transaction did not reauthorize")
	}
}
