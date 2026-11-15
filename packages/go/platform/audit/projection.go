package audit

import (
	"context"
	"slices"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"github.com/jackc/pgx/v5"
)

func text(v any) string {
	s, ok := v.(string)
	if ok {
		s, _ = ar.SafeText(s, 500)
	}
	return s
}
func historicalName(id, kind string, snapshot any, current string) Name {
	out := Name{ID: id, Type: kind, NameSource: "not_recorded"}
	if v := text(snapshot); v != "" {
		out.Name = v
		out.NameSource = "snapshot"
	} else if id != "" && current != "" {
		out.Name, _ = ar.SafeText(current, 100)
		out.NameSource = "current"
	}
	return out
}
func actionLabel(event string) string {
	if definition, ok := ar.Lookup(event); ok {
		return definition.Label
	}
	return "历史操作（未归类）"
}

func (s *Service) projectTx(ctx context.Context, tx pgx.Tx, a access.Actor, auth authorization, rows []rawEvent) ([]*Event, error) {
	if len(rows) > 100 {
		return nil, Problem("audit_query_budget_exceeded", 503)
	}
	refs := []SubjectRef{}
	for _, r := range rows {
		d, ok := ar.Lookup(r.EventType)
		if !ok || !scopeAllows(auth, r) {
			continue
		}
		if d.Metadata != "governance" && d.Metadata != "audit" {
			refs = append(refs, SubjectRef{r.ID, d.Module, r.ObjectType, r.ObjectID, r.ObjectRevision, r.Scope, d.Metadata})
		}
	}
	disclosures := map[string]Disclosure{}
	if len(refs) > 0 && s.disclosure != nil {
		bounded, cancel := context.WithTimeout(ctx, s.options.MetadataTimeout)
		var e error
		disclosures, e = s.disclosure.ResolveTx(bounded, tx, a, auth.Scope, refs)
		cancel()
		if e != nil {
			return nil, sourceUnavailable(e)
		}
	}
	out := make([]*Event, len(rows))
	for index, r := range rows {
		d, ok := ar.Lookup(r.EventType)
		if !ok || !scopeAllows(auth, r) {
			continue
		}
		details := r.Details
		if details == nil {
			details = map[string]any{}
		}
		context := r.Context
		if context == nil {
			context = map[string]any{}
		}
		changes := slices.Clone(r.Changes)
		if r.Schema == 0 {
			before, e := ar.Snapshot(details["before"], d.Fields)
			if e != nil {
				return nil, e
			}
			after, e := ar.Snapshot(details["after"], d.Fields)
			if e != nil {
				return nil, e
			}
			_, bp := details["before"]
			_, ap := details["after"]
			changes = ar.Diff(before, after, bp, ap)
			if actor, ok := details["actor"].(map[string]any); ok {
				context["actor_name"] = actor["display_name"]
			}
			if after, ok := after.(map[string]any); ok {
				context["object_label"] = after["name"]
			}
		}
		if changes == nil {
			changes = []ar.Change{}
		}
		actor := historicalName(r.ActorID, r.ActorType, context["actor_name"], r.ActorCurrent)
		actor.Username = text(context["actor_username"])
		if r.ActorType != "account" {
			actor.Ref = r.ExecutorRef
			if actor.Name == "" {
				actor.Name = map[string]string{"worker": "后台处理进程", "system": "系统", "maintenance": "维护操作", "release": "发布维护"}[r.ActorType]
			}
		}
		event := Event{ID: r.ID, Scope: auth.Scope, EventType: r.EventType, ActionLabel: actionLabel(r.EventType), Category: d.Category, Phase: d.Phase, Outcome: r.Outcome, Actor: actor, Initiator: historicalName(r.Initiator, "account", context["initiator_name"], r.InitiatorCurrent), RequestedBy: historicalName(r.RequestedBy, "account", context["requested_by_name"], r.RequestedByCurrent), Object: Object{Type: r.ObjectType, ID: r.ObjectID, Revision: r.ObjectRevision, LabelState: ar.NotRecorded, LabelSource: "not_recorded"}, OperationID: r.OperationID, Reason: text(r.Reason), OccurredAt: r.Occurred, RecordedAt: r.Recorded, TimeSource: r.TimeSource, Changes: changes, EvidenceQuality: r.Quality, MissingFields: slices.Clone(r.Missing), SourceState: "available", Actions: EventActions{r.OperationID != "", auth.Has("technical.read")}}
		if !slices.Contains([]string{"success", "failure", "denied", "conflict", "cancelled", "rate_limited", "unknown"}, event.Outcome) {
			event.Outcome = "unknown"
		}
		disclosed := d.Metadata == "governance" || d.Metadata == "audit" || disclosures[r.ID].Allowed
		if label := text(context["object_label"]); label != "" {
			event.Object.Label = label
			event.Object.LabelState = ar.Recorded
			event.Object.LabelSource = "snapshot"
		} else if disclosed && disclosures[r.ID].CurrentName != "" {
			event.Object.Label = text(disclosures[r.ID].CurrentName)
			event.Object.LabelState = ar.Recorded
			event.Object.LabelSource = "current"
		}
		if !disclosed {
			event.Object.Label = ""
			event.Object.LabelState = ar.Redacted
			event.Object.LabelSource = "restricted"
			event.Reason = "当前权限不可查看业务说明"
			for i := range event.Changes {
				event.Changes[i].Before = nil
				event.Changes[i].After = nil
				event.Changes[i].BeforeState = ar.Redacted
				event.Changes[i].AfterState = ar.Redacted
			}
		}
		notice := r.Visibility == "platform_tenant_notice" && auth.Scope.Kind == "tenant"
		if notice {
			event.Actor = Name{Type: "system", Name: "平台治理", NameSource: "snapshot"}
			event.Initiator = Name{Type: "account", NameSource: "not_recorded"}
			event.RequestedBy = event.Initiator
			event.Reason = ""
			event.OperationID = ""
			event.Changes = []ar.Change{}
			event.Actions = EventActions{}
			event.Object = Object{Type: "tenant", ID: auth.Scope.Tenant, LabelState: ar.NotRecorded, LabelSource: "not_recorded"}
			if after, ok := details["after"].(map[string]any); ok {
				if public, ok := after["public"].(map[string]any); ok {
					event.Reason = text(public["tenant_notice"])
					if status, ok := public["status"]; ok {
						event.Changes = ar.Diff(nil, map[string]any{"status": status}, false, true)
					}
				}
			}
		}
		if auth.Has("technical.read") && !notice {
			event.Technical = &Technical{SourceModule: r.SourceModule, SourceKey: r.SourceKey, OperationID: r.OperationID, RequestID: ar.SafeRequestID(r.RequestID), RequestKind: r.RequestKind, ErrorCode: r.ErrorCode, ExecutorKind: r.ExecutorKind, ExecutorRef: r.ExecutorRef}
			if auth.Scope.Kind == "platform" {
				event.Technical.IP = text(r.IP)
				event.Technical.UserAgent = text(r.UserAgent)
			}
		}
		if event.MissingFields == nil {
			event.MissingFields = []string{}
		}
		out[index] = &event
	}
	if auth.Has("technical.read") && s.trace != nil {
		if e := s.taskReferencesTx(ctx, tx, a, out); e != nil {
			return nil, e
		}
	}
	return out, nil
}

func (s *Service) taskReferencesTx(ctx context.Context, tx pgx.Tx, a access.Actor, events []*Event) error {
	ids := []string{}
	byID := map[string]*Event{}
	for _, event := range events {
		if event != nil && event.Technical != nil {
			ids = append(ids, event.ID)
			byID[event.ID] = event
			event.Technical.TaskReferences = []TaskReference{}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	bounded, cancel := context.WithTimeout(ctx, s.options.MetadataTimeout)
	defer cancel()
	rows, e := tx.Query(bounded, `SELECT id::text,event_id::text,source_module,target_kind,target_id,generation::text,scope_kind,COALESCE(tenant_id::text,''),COALESCE(project_id::text,'') FROM audit_event_links WHERE event_id=ANY($1::bigint[]) AND link_kind='task_execution' ORDER BY event_id,id`, ids)
	if e != nil {
		return e
	}
	links := []ar.Link{}
	owner := map[string]string{}
	for rows.Next() {
		var l ar.Link
		var id string
		if e = rows.Scan(&l.ID, &id, &l.Module, &l.TargetKind, &l.TargetID, &l.Generation, &l.Scope.Kind, &l.Scope.Tenant, &l.Scope.Project); e != nil {
			rows.Close()
			return e
		}
		l.Kind = "task_execution"
		l.Relation = "execution"
		links = append(links, l)
		owner[l.ID] = id
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for len(links) > 0 {
		batch := links[:min(len(links), 100)]
		resolved, e := s.trace.ResolveTx(bounded, tx, a, batch, true)
		if e != nil {
			return sourceUnavailable(e)
		}
		for _, l := range batch {
			if resolved[l.ID].Available {
				event := byID[owner[l.ID]]
				event.Technical.TaskReferences = append(event.Technical.TaskReferences, TaskReference{l.Module, l.TargetKind, l.TargetID, l.Generation})
			}
		}
		links = links[len(batch):]
	}
	return nil
}
