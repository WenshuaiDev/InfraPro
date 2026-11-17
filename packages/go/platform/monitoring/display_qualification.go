package monitoring

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dr "github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	"github.com/jackc/pgx/v5"
)

// QualifyDisplayMembersTx checks recursive input eligibility before a scope can
// expose a channel's identity or count, including configuration preview.
func (s *Service) QualifyDisplayMembersTx(ctx context.Context, tx pgx.Tx, a access.Actor, tenant, project string, d dr.Definition, members []dr.Member) error {
	if len(members) == 0 || s.displayQualification == nil {
		return nil
	}
	ids := (dr.Scope{Members: members}).IDs("channel")
	auth := authorization{Scope: ProjectScope(tenant, project), Account: a.ID, Subject: a.ReadSubject()}
	windows, e := authorizedWindowsTx(ctx, tx, auth, ids, "read")
	if e != nil {
		return e
	}
	if d.TimeWindow != nil {
		ceiling, e := dr.ParseWindow(d.TimeWindow, "/time_window")
		if e != nil {
			return e
		}
		for id, values := range windows {
			selected := []Window{}
			for _, w := range values {
				v, ok := dr.Intersect(access.ReadWindow{From: w.From, To: w.To}, ceiling)
				if ok {
					selected = append(selected, Window{From: v.From, To: v.To})
				}
			}
			windows[id] = selected
		}
	}
	return s.displayQualification(ctx, tx, a, ProjectScope(tenant, project), windows)
}
