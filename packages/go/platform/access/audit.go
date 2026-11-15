package access

import (
	"context"
	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
)

// LegacyAuditReader is supplied by application composition. Access owns no parallel audit query language.
type LegacyAuditReader func(context.Context, Actor, ar.Scope, Filter) (Page[AuditEvent], error)

func (s *Service) WithAuditReader(reader LegacyAuditReader) *Service {
	s.auditMu.Lock()
	s.auditReader = reader
	s.auditMu.Unlock()
	return s
}
func (s *Service) ReadAudit(ctx context.Context, a Actor, scope ar.Scope, f Filter) (Page[AuditEvent], error) {
	s.auditMu.RLock()
	reader := s.auditReader
	s.auditMu.RUnlock()
	if reader == nil {
		return Page[AuditEvent]{Items: []AuditEvent{}}, &Error{Code: "audit_protocol_unavailable", Message: "审计读取器尚未装配。", Status: 503}
	}
	return reader(ctx, a, scope, f)
}
func (s *Service) AuditEvents(ctx context.Context, a Actor, f Filter) (Page[AuditEvent], error) {
	return s.ReadAudit(ctx, a, ar.Scope{Kind: "platform"}, f)
}
