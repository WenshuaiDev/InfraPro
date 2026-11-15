package audit

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tasks"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ExportInput struct {
	Filter                Filter `json:"filter"`
	FieldProfile          string `json:"field_profile"`
	Reason                string `json:"reason"`
	Epoch                 string `json:"epoch"`
	AuthorizationRevision string `json:"authorization_revision"`
	ProjectionVersion     string `json:"projection_version"`
}
type Export struct {
	ID              string         `json:"id"`
	Scope           Scope          `json:"scope"`
	Generation      string         `json:"generation"`
	ControlRevision string         `json:"control_revision"`
	TaskID          string         `json:"task_id"`
	Owned           bool           `json:"owned"`
	State           string         `json:"state"`
	Phase           string         `json:"phase"`
	ResultState     string         `json:"result_state"`
	Filter          *Filter        `json:"filter,omitempty"`
	FieldProfile    string         `json:"field_profile"`
	Reason          string         `json:"reason,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	SnapshotAt      *time.Time     `json:"snapshot_at"`
	ExpiresAt       *time.Time     `json:"expires_at"`
	RowCount        *string        `json:"row_count"`
	SizeBytes       *string        `json:"size_bytes"`
	SHA256          string         `json:"sha256,omitempty"`
	ErrorCode       string         `json:"error_code,omitempty"`
	NextAction      string         `json:"next_action"`
	AllowedActions  []tasks.Action `json:"allowed_actions"`
	File            *ExportFile    `json:"file,omitempty"`
}
type ExportFile struct {
	Kind       string `json:"kind"`
	Scope      Scope  `json:"scope"`
	ExportID   string `json:"exportId"`
	Generation string `json:"generation"`
	Name       string `json:"name"`
	MediaType  string `json:"media_type"`
	SizeBytes  string `json:"size_bytes"`
	State      string `json:"state"`
}
type ExportPage struct {
	Items      []Export  `json:"items"`
	NextCursor string    `json:"next_cursor"`
	ObservedAt time.Time `json:"observed_at"`
}

type exportRecord struct {
	Export
	Owner, Membership, Vector, Projection, Epoch, Operation, RootOperation, SourceRevision, CurrentGeneration, RequestedBy string
	ObjectID, ObjectKey, ObjectState, SnapshotDigest                                                                       string
	Attempt                                                                                                                int
	Fence                                                                                                                  int64
	LeaseOwner                                                                                                             string
	LeaseUntil                                                                                                             *time.Time
}

const exportColumns = `x.id::text,x.scope_kind,COALESCE(x.tenant_id::text,''),COALESCE(x.project_id::text,''),x.owner_id::text,x.membership_key,x.filter,x.field_profile,x.reason,x.operation_id::text,x.current_generation::text,x.source_revision::text,x.control_revision::text,x.created_at,g.generation::text,g.requested_by::text,g.operation_id::text,g.authorization_vector,g.projection_version::text,g.restore_epoch::text,g.state,g.phase,g.fence,g.attempt,COALESCE(g.lease_owner,''),g.lease_until,g.error_code,g.snapshot_at,g.snapshot_rows::text,COALESCE(g.snapshot_digest,''),COALESCE(o.id::text,''),COALESCE(o.object_key,''),COALESCE(o.state,''),o.expires_at,o.size_bytes::text,COALESCE(o.sha256,'')`

func loadExportTx(ctx context.Context, tx pgx.Tx, id, generation string, lock bool) (exportRecord, error) {
	v := exportRecord{}
	suffix := ""
	if lock {
		suffix = " FOR UPDATE OF x,g"
	}
	e := tx.QueryRow(ctx, `SELECT `+exportColumns+` FROM audit_exports x JOIN audit_export_generations g ON g.export_id=x.id AND g.generation=COALESCE(NULLIF($2,'')::bigint,x.current_generation) LEFT JOIN audit_export_objects o ON o.id=g.object_id WHERE x.id=$1`+suffix, id, generation).Scan(&v.ID, &v.Scope.Kind, &v.Scope.Tenant, &v.Scope.Project, &v.Owner, &v.Membership, &v.Filter, &v.FieldProfile, &v.Reason, &v.RootOperation, &v.CurrentGeneration, &v.SourceRevision, &v.ControlRevision, &v.CreatedAt, &v.Generation, &v.RequestedBy, &v.Operation, &v.Vector, &v.Projection, &v.Epoch, &v.State, &v.Phase, &v.Fence, &v.Attempt, &v.LeaseOwner, &v.LeaseUntil, &v.ErrorCode, &v.SnapshotAt, &v.RowCount, &v.SnapshotDigest, &v.ObjectID, &v.ObjectKey, &v.ObjectState, &v.ExpiresAt, &v.SizeBytes, &v.SHA256)
	if errors.Is(e, pgx.ErrNoRows) {
		e = Problem("audit_not_found", 404)
	}
	return v, e
}
func commandError(e error) error {
	switch {
	case errors.Is(e, ar.ErrCommandRequired):
		return Problem("command_id_required", 428)
	case errors.Is(e, ar.ErrCommandConflict):
		return Problem("request_conflict", 409)
	case errors.Is(e, ar.ErrReceiptExpired):
		return Problem("command_receipt_expired", 410)
	case errors.Is(e, ar.ErrInvalid):
		return Problem("audit_filter_invalid", 400)
	}
	return e
}
func (s *Service) CreateExport(ctx context.Context, a access.Actor, scope Scope, command string, input ExportInput) (Export, error) {
	var id string
	e := s.transaction(ctx, a, scope, func(tx pgx.Tx, auth authorization) error {
		if !auth.Has("export") {
			return access.ErrDenied
		}
		if auth.Mode != "ready" {
			return Problem("audit_protocol_unavailable", 503)
		}
		if input.FieldProfile == "" {
			input.FieldProfile = "standard"
		}
		if input.FieldProfile != "standard" && (input.FieldProfile != "technical" || !auth.Has("technical.read")) {
			return access.ErrDenied
		}
		if input.Filter.Cursor != "" || input.Filter.Limit != 0 || utf8.RuneCountInString(strings.TrimSpace(input.Reason)) < 1 || utf8.RuneCountInString(input.Reason) > 500 {
			return Problem("audit_filter_invalid", 400)
		}
		f, e := s.normalizeFilter(input.Filter, auth, true)
		if e != nil {
			return e
		}
		f.Limit = 0
		input.Filter = f
		if input.Epoch != auth.Epoch {
			return Problem("audit_epoch_changed", 409)
		}
		if input.AuthorizationRevision != auth.Vector || input.ProjectionVersion != auth.Projection {
			return Problem("audit_export_invalidated", 409)
		}
		r, e := ar.BeginCommandTx(ctx, tx, ar.Command{Module: "audit", ActorID: a.ID, MembershipKey: auth.MembershipKey, Scope: scope, Action: "audit.export.requested", ID: command, Payload: input})
		if e != nil {
			return commandError(e)
		}
		if r.Replayed {
			id = r.ResultRef
			return nil
		}
		if s.objects == nil {
			return Problem("audit_storage_unavailable", 503)
		}
		if e = s.exportLimitTx(ctx, tx, a.ID, scope); e != nil {
			return e
		}
		id = uuid.NewString()
		_, e = tx.Exec(ctx, `INSERT INTO audit_exports(id,scope_kind,tenant_id,project_id,scope_key,owner_id,membership_key,filter,field_profile,reason,operation_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, id, scope.Kind, taskrecord.Null(scope.Tenant), taskrecord.Null(scope.Project), scope.Key(), a.ID, auth.MembershipKey, ar.JSON(f), input.FieldProfile, safeReason(input.Reason), r.OperationID)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO audit_export_generations(export_id,generation,requested_by,operation_id,authorization_vector,projection_version,restore_epoch,state) VALUES($1,1,$2,$3,$4,$5,$6,'queued')`, id, a.ID, r.OperationID, auth.Vector, auth.Projection, auth.Epoch)
		if e != nil {
			return e
		}
		v, e := loadExportTx(ctx, tx, id, "", true)
		if e != nil {
			return e
		}
		if e = s.exportFactTx(ctx, tx, v, "audit.export.requested", a.ID, "account", input.Reason); e != nil {
			return e
		}
		if e = s.indexExportTx(ctx, tx, v, a.ID, "accepted"); e != nil {
			return e
		}
		return ar.CompleteCommandTx(ctx, tx, r, map[string]string{"export_id": id, "generation": "1", "task_id": exportSource(id).ID()}, "accepted", id)
	})
	var unknown *ar.CommitUnknown
	if errors.As(e, &unknown) {
		probe, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		original := e
		e = s.transaction(probe, a, scope, func(tx pgx.Tx, auth authorization) error {
			if !auth.Has("export") {
				return access.ErrDenied
			}
			r, err := ar.BeginCommandTx(probe, tx, ar.Command{Module: "audit", ActorID: a.ID, MembershipKey: auth.MembershipKey, Scope: scope, Action: "audit.export.requested", ID: command, Payload: input})
			if err != nil {
				return commandError(err)
			}
			if !r.Replayed {
				return original
			}
			id = r.ResultRef
			return nil
		})
		if e != nil {
			return Export{}, Problem("operation_status_unknown", 503)
		}
		return s.Export(probe, a, scope, id, "")
	}
	if e != nil {
		return Export{}, e
	}
	return s.Export(ctx, a, scope, id, "")
}
func (s *Service) exportLimitTx(ctx context.Context, tx pgx.Tx, owner string, scope Scope) error {
	if _, e := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(834327119)`); e != nil {
		return e
	}
	var mine, space, all int
	e := tx.QueryRow(ctx, `SELECT count(*) FILTER(WHERE x.owner_id=$1),count(*) FILTER(WHERE x.scope_key=$2),count(*) FROM audit_exports x JOIN audit_export_generations g ON g.export_id=x.id AND g.generation=x.current_generation WHERE g.state IN ('queued','running','retry_wait','cancelling')`, owner, scope.Key()).Scan(&mine, &space, &all)
	if e == nil && (mine >= s.options.ActivePerAccount || space >= s.options.ActivePerScope || all >= s.options.ActivePerInstance) {
		return Problem("audit_export_limit", 429)
	}
	return e
}
func exportSource(id string) taskrecord.SourceRef {
	return taskrecord.SourceRef{Module: "audit", Kind: "export", RootID: id}
}
func (s *Service) presentExport(v exportRecord, a access.Actor, auth authorization) (Export, error) {
	if v.Scope != auth.Scope || v.Owner != a.ID && !auth.Has("exports.manage") {
		return Export{}, Problem("audit_not_found", 404)
	}
	o := v.Export
	o.Owned = v.Owner == a.ID
	o.TaskID = exportSource(v.ID).ID()
	o.AllowedActions = []tasks.Action{}
	o.NextAction = "wait"
	o.ResultState = "preparing"
	valid := v.Vector == auth.Vector && v.Epoch == auth.Epoch && v.Projection == auth.Projection && v.Membership == auth.MembershipKey
	if taskrecord.Terminal(v.State) {
		o.ResultState = "none"
		o.NextAction = "view_details"
	}
	if v.State == "succeeded" {
		o.ResultState = v.ObjectState
		if o.ResultState == "" {
			o.ResultState = "missing"
		}
		if v.ExpiresAt != nil && !v.ExpiresAt.After(s.now()) {
			o.ResultState = "expired"
		}
		if !o.Owned || !auth.Has("export") {
			o.ResultState = "restricted"
		} else if !valid {
			o.ResultState = "invalidated"
		}
		o.NextAction = "recreate"
		if o.ResultState == "available" {
			o.NextAction = "download"
		}
	}
	canCancel := v.Generation == v.CurrentGeneration && taskrecord.Active(v.State) && v.State != "cancelling" && (o.Owned && auth.Has("export") || auth.Has("exports.manage"))
	canRetry := v.Generation == v.CurrentGeneration && (v.State == "failed" || v.State == "cancelled") && o.Owned && auth.Has("export")
	o.AllowedActions = append(o.AllowedActions, tasks.Action{Kind: "cancel", Enabled: canCancel, RequiresReason: !o.Owned}, tasks.Action{Kind: "retry", Enabled: canRetry, RequiresReason: true}, tasks.Action{Kind: "download", Enabled: o.ResultState == "available"}, tasks.Action{Kind: "recreate", Enabled: o.Owned && auth.Has("export")})
	if !o.Owned {
		o.Filter = nil
		o.Reason = ""
		o.SHA256 = ""
		o.RowCount = nil
		o.SizeBytes = nil
	}
	if o.ResultState == "available" && o.SizeBytes != nil {
		o.File = &ExportFile{Kind: "audit.export", Scope: v.Scope, ExportID: v.ID, Generation: v.Generation, Name: "审计导出.csv", MediaType: "text/csv", SizeBytes: *o.SizeBytes, State: o.ResultState}
	}
	return o, nil
}
func (s *Service) Export(ctx context.Context, a access.Actor, scope Scope, id, generation string) (Export, error) {
	o := Export{}
	if !ar.ID(id) || generation != "" && !ar.Number(generation, 1) {
		return o, Problem("audit_reference_invalid", 400)
	}
	e := s.transaction(ctx, a, scope, func(tx pgx.Tx, auth authorization) error {
		v, e := loadExportTx(ctx, tx, id, generation, false)
		if e != nil {
			return e
		}
		o, e = s.presentExport(v, a, auth)
		return e
	})
	return o, e
}

type ExportFilter struct {
	From   string `json:"from,omitempty"`
	To     string `json:"to,omitempty"`
	State  string `json:"state,omitempty"`
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

func (s *Service) Exports(ctx context.Context, a access.Actor, scope Scope, input ExportFilter, managed bool) (ExportPage, error) {
	o := ExportPage{Items: []Export{}, ObservedAt: s.now()}
	e := s.transaction(ctx, a, scope, func(tx pgx.Tx, auth authorization) error {
		if managed && !auth.Has("exports.manage") {
			return access.ErrDenied
		}
		limit := input.Limit
		if limit == 0 {
			limit = s.options.PageSize
		}
		if limit < 1 || limit > s.options.MaxPageSize {
			return Problem("audit_filter_invalid", 400)
		}
		if input.State != "" && !slices.Contains([]string{"queued", "running", "retry_wait", "cancelling", "succeeded", "failed", "cancelled"}, input.State) {
			return Problem("audit_filter_invalid", 400)
		}
		c := cursor{Actor: a.ID, Scope: scope.Key(), Vector: auth.Vector, Filter: ar.Hash([]any{limit, managed, input.From, input.To, input.State}), Upper: s.now(), Purpose: "exports"}
		if input.Cursor != "" {
			v, e := parseCursor(input.Cursor, auth.Secret)
			if e != nil {
				return e
			}
			if v.Actor != c.Actor || v.Scope != c.Scope || v.Vector != c.Vector || v.Filter != c.Filter || v.Purpose != c.Purpose {
				return Problem("audit_cursor_stale", 409)
			}
			c = v
		}
		window := Filter{From: input.From, To: input.To, Limit: limit}
		if window.From == "" {
			window.From = c.Upper.Add(-s.options.DefaultWindow).Format(time.RFC3339Nano)
		}
		if window.To == "" {
			window.To = c.Upper.Format(time.RFC3339Nano)
		}
		normalized, err := s.normalizeFilter(window, auth, false)
		if err != nil {
			return err
		}
		var at any
		if !c.At.IsZero() {
			at = c.At
		}
		rows, e := tx.Query(ctx, `SELECT e.id::text FROM audit_exports e JOIN audit_export_generations g ON g.export_id=e.id AND g.generation=e.current_generation WHERE e.scope_key=$1 AND ($2 OR e.owner_id=$3) AND e.created_at<=$4 AND ($5::timestamptz IS NULL OR (e.created_at,e.id)<($5,$6::uuid)) AND e.created_at >= $8 AND e.created_at < $9 AND ($10='' OR g.state=$10) ORDER BY e.created_at DESC,e.id DESC LIMIT $7`, scope.Key(), managed, a.ID, c.Upper, at, taskrecord.Null(c.ID), limit+1, normalized.From, normalized.To, input.State)
		if e != nil {
			return e
		}
		ids, e := pgx.CollectRows(rows, pgx.RowTo[string])
		if e != nil {
			return e
		}
		for _, id := range ids {
			if len(o.Items) == limit {
				last := o.Items[len(o.Items)-1]
				c.At = last.CreatedAt
				c.ID = last.ID
				o.NextCursor = signCursor(c, auth.Secret)
				break
			}
			v, e := loadExportTx(ctx, tx, id, "", false)
			if e != nil {
				return e
			}
			p, e := s.presentExport(v, a, auth)
			if e != nil {
				return e
			}
			o.Items = append(o.Items, p)
		}
		return nil
	})
	return o, e
}
func (s *Service) exportFactTx(ctx context.Context, tx pgx.Tx, v exportRecord, event, actor, actorType, reason string) error {
	operation, e := ar.TransactionOperationTx(ctx, tx)
	if e != nil {
		return e
	}
	key := v.ID + ":" + v.Generation + ":" + event
	if event == "audit.export.cancel.accepted" {
		key += ":" + v.ControlRevision
	}
	f := ar.Fact{Module: "audit", SourceKey: key, Effect: "export", Event: event, Scope: v.Scope, ActorID: actor, ActorType: actorType, Initiator: v.Owner, RequestedBy: v.RequestedBy, OperationID: operation, ObjectType: "audit_export", ObjectID: v.ID, ObjectRevision: v.Generation, Outcome: "success", Reason: reason, ErrorCode: v.ErrorCode, RequestKind: "http", Changes: ar.Diff(nil, map[string]string{"state": v.State, "generation": v.Generation}, false, true), Links: []ar.Link{{Kind: "task_execution", Module: "audit", TargetKind: "export", TargetID: v.ID, Generation: v.Generation, Relation: "execution", Scope: v.Scope}, {Kind: "domain_file", Module: "audit", TargetKind: "audit_export", TargetID: v.ID, Generation: v.Generation, Relation: "output", Scope: v.Scope}}}
	if operation != v.Operation {
		f.ParentOperationID = v.Operation
	}
	if actorType != "account" {
		f.ActorID = ""
		f.RequestKind = "worker"
		f.ExecutorKind = "delegated"
		f.ExecutorRef = s.options.NodeID
	}
	if v.State == "failed" {
		f.Outcome = "failure"
	}
	if v.State == "cancelled" {
		f.Outcome = "cancelled"
	}
	_, e = ar.AppendTx(ctx, tx, f)
	return e
}
func (s *Service) indexExportTx(ctx context.Context, tx pgx.Tx, v exportRecord, actor, event string) error {
	phase := v.Phase
	if v.State == "succeeded" {
		phase = "completed"
	}
	if v.State == "failed" || v.State == "cancelled" {
		phase = v.State
	}
	c := taskrecord.Change{SourceRef: exportSource(v.ID), Type: "audit.export", Scope: taskrecord.Scope{Kind: v.Scope.Kind, Tenant: v.Scope.Tenant, Project: v.Scope.Project}, Originator: v.Owner, OriginKind: "person", EventActor: actor, RequestedBy: v.RequestedBy, ExecutorKind: "delegated", ExecutorRef: "audit-export", Generation: v.Generation, Revision: v.SourceRevision, ControlRevision: v.ControlRevision, State: v.State, SourceState: v.State, Phase: phase, Progress: taskrecord.Progress{Completed: "0", Unit: "rows"}, ErrorCode: v.ErrorCode, InputRef: map[string]string{"export_id": v.ID, "generation": v.Generation}, InputHash: ar.Hash([]any{v.Filter, v.FieldProfile, v.Generation}), ResultRef: map[string]string{}, CreatedAt: v.CreatedAt, ObservedAt: s.now(), Fence: strconv.FormatInt(v.Fence, 10), Attempt: v.Attempt, Event: event}
	if v.RowCount != nil {
		c.Progress.Completed = *v.RowCount
		c.Progress.Total = v.RowCount
	}
	if v.ObjectID != "" {
		c.ResultRef = map[string]string{"export_id": v.ID, "generation": v.Generation, "object_id": v.ObjectID}
	}
	if _, e := taskrecord.ApplyTx(ctx, tx, c); e != nil {
		return e
	}
	_, e := tx.Exec(ctx, `UPDATE audit_exports SET task_snapshot=$2,updated_at=clock_timestamp() WHERE id=$1`, v.ID, ar.JSON(c))
	return e
}

func safeReason(s string) string { v, _ := ar.SafeText(s, 500); return v }
