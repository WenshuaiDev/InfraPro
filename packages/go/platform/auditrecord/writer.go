package auditrecord

import (
	"context"
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var safeCode = regexp.MustCompile(`^[A-Za-z0-9_.:/-]*$`)
var safeRequest = regexp.MustCompile(`^[A-Za-z0-9_.:-]{0,100}$`)

func Visibility(d Definition, scope Scope, entry string) (string, error) {
	switch d.ScopeRule {
	case "platform":
		if scope.Tenant != "" || scope.Project != "" {
			return "", ErrInvalid
		}
		return "platform", nil
	case "platform_tenant":
		if !ID(scope.Tenant) || scope.Project != "" {
			return "", ErrInvalid
		}
		return "platform", nil
	case "notice":
		if !ID(scope.Tenant) || scope.Project != "" {
			return "", ErrInvalid
		}
		return "platform_tenant_notice", nil
	case "governance":
		if !ID(scope.Tenant) || scope.Project != "" {
			return "", ErrInvalid
		}
		return "tenant_project_governance", nil
	case "project":
		if scope.Kind != "project" || !scope.Valid() {
			return "", ErrInvalid
		}
		return "project", nil
	case "tenant":
		if scope.Kind != "tenant" || !scope.Valid() {
			return "", ErrInvalid
		}
		return "tenant", nil
	case "entry":
		if entry == "platform" && scope.Project == "" {
			return "platform", nil
		}
		fallthrough
	case "business_or_platform":
		if scope.Kind == "platform" && scope.Valid() {
			return "platform", nil
		}
		fallthrough
	case "business":
		if scope.Kind == "project" && scope.Valid() {
			return "project", nil
		}
		if scope.Kind == "tenant" && scope.Valid() {
			return "tenant", nil
		}
	}
	return "", ErrInvalid
}
func prepare(f Fact) (Fact, error) {
	d, ok := Lookup(f.Event)
	if !ok {
		return f, ErrUnregistered
	}
	if f.Module == "" {
		f.Module = d.Module
	}
	if f.Module != d.Module {
		return f, ErrInvalid
	}
	if !f.Scope.Valid() {
		return f, ErrInvalid
	}
	if f.Visibility == "" {
		var e error
		f.Visibility, e = Visibility(d, f.Scope, f.Scope.Kind)
		if e != nil {
			return f, e
		}
	}
	allowed, e := Visibility(d, f.Scope, f.Scope.Kind)
	if e != nil {
		return f, e
	}
	if f.Visibility != allowed && !(d.ScopeRule == "entry" && f.Visibility == "platform" && f.Scope.Project == "") {
		return f, ErrInvalid
	}
	f.Category = d.Category
	f.Phase = d.Phase
	if f.ActorType == "" {
		f.ActorType = "account"
	}
	if f.Outcome == "" {
		f.Outcome = "success"
	}
	if !slices.Contains([]string{"account", "worker", "system", "maintenance", "release"}, f.ActorType) || !slices.Contains([]string{"success", "failure", "denied", "conflict", "cancelled", "rate_limited", "unknown"}, f.Outcome) {
		return f, ErrInvalid
	}
	if f.ActorType != "account" && f.ActorID != "" {
		return f, ErrInvalid
	}
	if f.RequestKind == "" {
		f.RequestKind = "http"
	}
	if !slices.Contains([]string{"http", "cli", "worker", "command_legacy", "legacy_unknown"}, f.RequestKind) {
		return f, ErrInvalid
	}
	for _, id := range []string{f.ActorID, f.AccountID, f.Initiator, f.RequestedBy, f.OperationID, f.ParentOperationID} {
		if id != "" && !ID(id) {
			return f, ErrInvalid
		}
	}
	if f.SourceKey == "" || len(f.SourceKey) > 240 || f.Effect == "" || len(f.Effect) > 80 || len(f.ObjectType) > 80 || !safeCode.MatchString(f.ObjectType) || len(f.ObjectID) > 128 || !safeCode.MatchString(f.ObjectID) || !safeRequest.MatchString(f.RequestID) || len(f.ExecutorKind) > 40 || len(f.ExecutorRef) > 160 || len(f.ObjectRevision) > 80 || len(f.ErrorCode) > 120 || !safeCode.MatchString(f.ErrorCode) || !safeCode.MatchString(f.ExecutorRef) || !safeCode.MatchString(f.ExecutorKind) || utf8.RuneCountInString(f.Reason) > 500 {
		return f, ErrInvalid
	}
	f.Reason, _ = SafeText(f.Reason, 500)
	f.IP, _ = SafeText(f.IP, 100)
	f.UserAgent, _ = SafeText(f.UserAgent, 300)
	if f.Visibility != "platform" && f.Visibility != "platform_tenant_notice" {
		f.IP = ""
		f.UserAgent = ""
	}
	if f.Quality == "" {
		f.Quality = "native"
	}
	if !slices.Contains([]string{"native", "legacy_partial", "source_handoff"}, f.Quality) {
		return f, ErrInvalid
	}
	if f.Missing == nil {
		f.Missing = []string{}
	}
	for _, field := range f.Missing {
		if len(field) > 80 || !safeCode.MatchString(field) {
			return f, ErrInvalid
		}
	}
	if f.Changes == nil {
		f.Changes = []Change{}
	}
	if len(f.Changes) > 128 || len(f.Links) > 32 {
		return f, ErrInvalid
	}
	for i, c := range f.Changes {
		if !slices.Contains(d.Fields, c.Key) && c.Key != "value" {
			return f, ErrInvalid
		}
		if !slices.Contains([]string{"text", "boolean", "number", "enum", "time", "set", "reference", "revision", "summary"}, c.Type) {
			return f, ErrInvalid
		}
		for _, s := range []ValueState{c.BeforeState, c.AfterState} {
			if !slices.Contains([]ValueState{Recorded, NotRecorded, Redacted, Truncated}, s) {
				return f, ErrInvalid
			}
		}
		c.Label = Label(c.Key)
		c.Before, e = Snapshot(c.Before, append(slices.Clone(d.Fields), "value", "value_state", "sample", "count", "sha256"))
		if e != nil {
			return f, e
		}
		c.After, e = Snapshot(c.After, append(slices.Clone(d.Fields), "value", "value_state", "sample", "count", "sha256"))
		if e != nil {
			return f, e
		}
		if c.BeforeState == NotRecorded || c.BeforeState == Redacted {
			c.Before = nil
		}
		if c.AfterState == NotRecorded || c.AfterState == Redacted {
			c.After = nil
		}
		f.Changes[i] = c
	}
	if f.Context == nil {
		f.Context = map[string]any{}
	}
	// Context is a separate shallow contract; it cannot carry arbitrary payloads.
	contextFields := []string{"actor_name", "actor_username", "initiator_name", "requested_by_name", "object_label", "object_label_source", "actor_name_source", "before_state", "action", "transfer_id", "transfer_outcome", "sent_bytes", "method", "range", "public_message", "time_source", "reason_sha256", "reason_length"}
	v, e := Snapshot(f.Context, contextFields)
	if e != nil {
		return f, e
	}
	f.Context = v.(map[string]any)
	if f.Details == nil {
		f.Details = map[string]any{}
	}
	v, e = Snapshot(f.Details, append(slices.Clone(d.Fields), "value", "value_state", "sample", "actor", "username", "display_name", "task", "task_id", "before_state", "after_state", "original_actor", "requested_by"))
	if e != nil {
		return f, e
	}
	f.Details = v.(map[string]any)
	if len(JSON(f.Changes)) > 32768 || len(JSON(f.Context)) > 8192 || len(JSON(f.Details)) > 32768 {
		return f, ErrInvalid
	}
	return f, nil
}

func AppendTx(ctx context.Context, tx pgx.Tx, input Fact) (EventRef, error) {
	out := EventRef{}
	f, e := prepare(input)
	if e != nil {
		return out, e
	}
	var mode string
	if e = tx.QueryRow(ctx, `SELECT mode FROM audit_protocol WHERE id=1 AND version=1 FOR SHARE`).Scan(&mode); e != nil {
		return out, e
	}
	if mode == "reconciling" || mode == "migrating" {
		return out, ErrProtocol
	}
	if f.OperationID == "" {
		f.OperationID = uuid.NewSHA1(uuid.NameSpaceOID, []byte("audit-operation:"+f.Module+":"+f.SourceKey)).String()
	}
	if f.ParentOperationID != "" {
		if e = validateParent(ctx, tx, f.OperationID, f.ParentOperationID); e != nil {
			return out, e
		}
	}
	// Default time is excluded from the fingerprint: retrying a source fact must
	// not turn the writer's clock into different source evidence.
	fingerprint := Hash(f)
	var existing string
	e = tx.QueryRow(ctx, `SELECT id::text,fingerprint,operation_id::text FROM account_security_events WHERE source_module=$1 AND source_key=$2 AND effect_key=$3`, f.Module, f.SourceKey, f.Effect).Scan(&out.ID, &existing, &out.OperationID)
	if e == nil {
		if existing != fingerprint {
			return out, ErrConflict
		}
		return out, finishLinksTx(ctx, tx, out, f)
	}
	if e != pgx.ErrNoRows {
		return out, e
	}
	// Source operations are append-only, just like command operations. A single
	// committed business transaction may contain effects in several scopes.
	_, e = tx.Exec(ctx, `INSERT INTO audit_operations(id,source_module,scope_kind,tenant_id,project_id,actor_id,scope_key,action,parent_operation_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(id) DO NOTHING`, f.OperationID, f.Module, f.Scope.Kind, null(f.Scope.Tenant), null(f.Scope.Project), null(f.ActorID), f.Scope.Key(), f.Event, null(f.ParentOperationID))
	if e != nil {
		return out, e
	}
	var at any
	if !f.OccurredAt.IsZero() {
		at = f.OccurredAt.UTC()
	}
	retention := f.Scope.Kind
	if f.Visibility == "platform" || f.Visibility == "platform_tenant_notice" {
		retention = "platform"
	}
	e = tx.QueryRow(ctx, `INSERT INTO account_security_events(event_type,actor_id,account_id,object_id,outcome,reason,source_ip,user_agent,request_id,actor_type,object_type,details,tenant_id,actor_scope,project_id,schema_version,source_module,source_key,effect_key,fingerprint,operation_id,parent_operation_id,request_id_kind,visibility_class,category,phase,occurred_at,time_source,initiator_id,requested_by,executor_kind,executor_ref,object_revision,error_code,changes,safe_context,evidence_quality,missing_fields,retention_class,retention_policy_version,retain_until)
 SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,1,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,COALESCE($26::timestamptz,transaction_timestamp()),'native',$27,$28,$29,$30,$31,$32,$33,$34,$35,$36,p.class,p.version,CASE WHEN p.keep_seconds IS NOT NULL THEN COALESCE($26::timestamptz,transaction_timestamp())+p.keep_seconds*interval '1 second' END
 FROM audit_retention_policies p WHERE p.class=$37 ORDER BY p.version DESC LIMIT 1
 ON CONFLICT(source_module,source_key,effect_key) WHERE source_key IS NOT NULL DO NOTHING RETURNING id::text,operation_id::text`, f.Event, null(f.ActorID), null(f.AccountID), f.ObjectID, f.Outcome, f.Reason, f.IP, f.UserAgent, f.RequestID, f.ActorType, f.ObjectType, JSON(f.Details), null(f.Scope.Tenant), f.Scope.Kind, null(f.Scope.Project), f.Module, f.SourceKey, f.Effect, fingerprint, f.OperationID, null(f.ParentOperationID), f.RequestKind, f.Visibility, f.Category, f.Phase, at, null(f.Initiator), null(f.RequestedBy), f.ExecutorKind, f.ExecutorRef, f.ObjectRevision, f.ErrorCode, JSON(f.Changes), JSON(f.Context), f.Quality, f.Missing, retention).Scan(&out.ID, &out.OperationID)
	if e == pgx.ErrNoRows {
		e = tx.QueryRow(ctx, `SELECT id::text,fingerprint,operation_id::text FROM account_security_events WHERE source_module=$1 AND source_key=$2 AND effect_key=$3`, f.Module, f.SourceKey, f.Effect).Scan(&out.ID, &existing, &out.OperationID)
		if e == nil && existing != fingerprint {
			return out, ErrConflict
		}
	}
	if e != nil {
		return out, e
	}
	return out, finishLinksTx(ctx, tx, out, f)
}
func validateParent(ctx context.Context, tx pgx.Tx, id, parent string) error {
	if id == parent {
		return ErrInvalid
	}
	var found, bad bool
	e := tx.QueryRow(ctx, `WITH RECURSIVE chain AS (SELECT id,parent_operation_id,1 AS depth FROM audit_operations WHERE id=$1 UNION ALL SELECT p.id,p.parent_operation_id,c.depth+1 FROM audit_operations p JOIN chain c ON p.id=c.parent_operation_id WHERE c.depth<=32) SELECT EXISTS(SELECT 1 FROM chain),EXISTS(SELECT 1 FROM chain WHERE id=$2 OR depth>32)`, parent, id).Scan(&found, &bad)
	if e == nil && (!found || bad) {
		e = ErrInvalid
	}
	return e
}

// TransactionOperationTx groups actual effects in the caller's transaction,
// independently of untrusted transport request IDs and repeated object names.
func TransactionOperationTx(ctx context.Context, tx pgx.Tx) (string, error) {
	var id string
	e := tx.QueryRow(ctx, `SELECT COALESCE(current_setting('infrapro.audit_operation',true),'')`).Scan(&id)
	if e != nil {
		return "", e
	}
	if id == "" {
		id = uuid.NewString()
		_, e = tx.Exec(ctx, `SELECT set_config('infrapro.audit_operation',$1,true)`, id)
	}
	return id, e
}
func CommandTx(ctx context.Context, tx pgx.Tx) (CommandContext, error) {
	var c CommandContext
	var raw string
	e := tx.QueryRow(ctx, `SELECT COALESCE(current_setting('infrapro.audit_command',true),'')`).Scan(&raw)
	if e == nil && raw != "" {
		e = json.Unmarshal([]byte(raw), &c)
	}
	return c, e
}

func LinkTx(ctx context.Context, tx pgx.Tx, ref EventRef, links []Link) error {
	if !Number(ref.ID, 1) || len(links) > 32 {
		return ErrInvalid
	}
	for _, l := range links {
		if !slices.Contains([]string{"business", "task_execution", "file_revision", "domain_file", "notification_event", "caused_by"}, l.Kind) || !slices.Contains([]string{"subject", "input", "output", "execution", "notification", "caused_by", "retry_of"}, l.Relation) || !l.Scope.Valid() || len(l.Module) > 64 || !safeCode.MatchString(l.Module) || len(l.TargetKind) > 80 || !safeCode.MatchString(l.TargetKind) || !safeCode.MatchString(l.TargetID) || len(l.TargetID) > 80 || l.TargetID == "" {
			return ErrInvalid
		}
		if l.Generation != "" && !Number(l.Generation, 1) || l.Kind == "task_execution" && l.Generation == "" || l.Kind == "file_revision" && !ID(l.Revision) {
			return ErrInvalid
		}
		var tenant, project, visibility string
		if e := tx.QueryRow(ctx, `SELECT COALESCE(tenant_id::text,''),COALESCE(project_id::text,''),visibility_class FROM account_security_events WHERE id=$1`, ref.ID).Scan(&tenant, &project, &visibility); e != nil {
			return e
		}
		if tenant != l.Scope.Tenant || project != l.Scope.Project {
			return ErrInvalid
		}
		if l.Kind == "notification_event" {
			if !ID(l.TargetID) {
				return ErrInvalid
			}
			var exists bool
			e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM notification_events WHERE id=$1 AND COALESCE(tenant_id::text,'')=$2 AND COALESCE(project_id::text,'')=$3)`, l.TargetID, tenant, project).Scan(&exists)
			if e != nil {
				return e
			}
			if !exists {
				return ErrInvalid
			}
		}
		if l.Kind == "caused_by" {
			if !Number(l.TargetID, 1) || l.TargetID == ref.ID {
				return ErrInvalid
			}
			var exists, cycle bool
			e := tx.QueryRow(ctx, `WITH RECURSIVE chain AS (SELECT id::text AS id,1 AS depth FROM account_security_events WHERE id=$1 AND COALESCE(tenant_id::text,'')=$3 AND COALESCE(project_id::text,'')=$4 UNION ALL SELECT l.target_id,c.depth+1 FROM chain c JOIN audit_event_links l ON l.event_id::text=c.id AND l.link_kind='caused_by' WHERE c.depth<=32) SELECT EXISTS(SELECT 1 FROM chain),EXISTS(SELECT 1 FROM chain WHERE id=$2 OR depth>32)`, l.TargetID, ref.ID, tenant, project).Scan(&exists, &cycle)
			if e != nil {
				return e
			}
			if !exists || cycle {
				return ErrInvalid
			}
		}
		l.ID = ""
		_, e := tx.Exec(ctx, `INSERT INTO audit_event_links(event_id,link_kind,source_module,target_kind,target_id,generation,revision,relation,scope_kind,tenant_id,project_id,link_key) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) ON CONFLICT(event_id,link_key) DO NOTHING`, ref.ID, l.Kind, l.Module, l.TargetKind, l.TargetID, null(l.Generation), null(l.Revision), l.Relation, l.Scope.Kind, null(l.Scope.Tenant), null(l.Scope.Project), Hash(l))
		if e != nil {
			return e
		}
	}
	return nil
}

func SafeRequestID(v string) string {
	if !safeRequest.MatchString(v) {
		return ""
	}
	return strings.TrimSpace(v)
}
