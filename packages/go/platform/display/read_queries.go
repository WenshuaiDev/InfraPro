package display

import (
	"context"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	"github.com/jackc/pgx/v5"
)

func (s *Service) WithReads(reads *displayread.Service) *Service {
	s.reads = reads
	reads.WithAuthority(func(ctx context.Context, owner displayread.Owner) (displayread.Authority, error) {
		var session Session
		e := s.access.Transact(ctx, func(tx pgx.Tx) error {
			var e error
			session, e = scanSession(tx.QueryRow(ctx, sessionSelect+` WHERE s.id=$1`, owner.Session))
			return e
		})
		if e != nil {
			return displayread.Authority{}, e
		}
		a := s.ReadAuthority(session)
		if a.Owner != owner {
			return displayread.Authority{}, hidden()
		}
		return a, nil
	})
	p := ProviderFunc(func(ctx context.Context, subject *access.ReadSubject, call Call) (any, error) {
		var session Session
		e := s.access.Transact(ctx, func(tx pgx.Tx) error {
			var e error
			session, e = scanSession(tx.QueryRow(ctx, sessionSelect+` WHERE s.subject_id=$1 ORDER BY s.created_at DESC LIMIT 1`, subject.ID))
			return e
		})
		if e != nil {
			return nil, e
		}
		return reads.Read(ctx, s.ReadAuthority(session), call.Method, call.Params, call.Generation)
	})
	methods := append([]string{"views.list", "views.describe", "queries.get", "queries.cancel", "queries.release", "queries.drilldown"}, reads.Methods()...)
	for _, method := range methods {
		cap := methodCapabilities[method]
		if s.providers[cap] == nil {
			if e := s.Register(cap, p); e != nil {
				panic(e)
			}
		}
	}
	return s
}
func (s *Service) readQuery(ctx context.Context, v Session, call Call) (Envelope, error) {
	if s.reads == nil {
		return Envelope{}, problem("capability_unavailable", "命名读取服务未安装。", 409)
	}
	done, e := s.budget.Take(map[string]int{"tenant:" + v.Tenant: s.options.MaxQueries * 8, "project:" + v.Project: s.options.MaxQueries * 4, "instance:" + v.InstanceID: s.options.MaxQueries * 2, "session:" + v.ID: s.options.MaxQueries})
	if e != nil {
		return Envelope{}, e
	}
	defer done()
	r, e := s.reads.Read(ctx, s.ReadAuthority(v), call.Method, call.Params, call.Generation)
	return Envelope{NamedRead: true, State: r.State, Data: r.Data, ReadAt: r.ReadAt, Authorization: r.Authorization, Generation: r.Generation, SourceVector: r.SourceVector, Query: r.Query, IncludedFields: r.IncludedFields, RetryAfter: r.RetryAfter}, displayread.ReadError(e)
}
