// Package notificationevent is the transaction-only producer contract. It has no
// dependency on access, identity, a business domain, or the inbox reader.
package notificationevent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Definition struct {
	Category, Title, SafeTitle string
	Mandatory                  bool
}

var Catalog = map[string]Definition{
	"security.changed":   {"security", "账号安全设置已变更", "账号安全设置已变更", true},
	"signup.used":        {"organization", "开户邀请已使用", "你发出的开户邀请已使用", true},
	"invitation.changed": {"organization", "租户邀请状态更新", "你的租户邀请状态已更新", true},
	"membership.changed": {"organization", "成员资格已变更", "你的工作空间资格已变更", true},
	"role.changed":       {"organization", "角色权限已变更", "你的角色权限已变更", true},
	"lifecycle.changed":  {"organization", "工作空间状态已变更", "你参与的工作空间状态已变更", true},
	"grant.changed":      {"updates", "项目资源授权已变更", "项目资源授权已变更", false},
	"task.result":        {"tasks", "任务结果已更新", "你发起的任务已有结果", false},
	"derived.condition":  {"updates", "自动计算状态更新", "自动计算状态更新", false},
	"spatial.result":     {"tasks", "空间资料转换结果", "你发起的转换已有结果", false},
	"spatial.published":  {"updates", "空间资料发布更新", "空间资料发布更新", false},
	"display.package":    {"tasks", "大屏包处理结果", "你上传的大屏包已有结果", false},
	"display.changed":    {"updates", "大屏管理状态更新", "大屏管理状态更新", false},
	"display.condition":  {"updates", "大屏发布包状态更新", "大屏发布包状态更新", false},
}

type Scope struct {
	Kind    string `json:"kind"`
	Tenant  string `json:"tenant_id,omitempty"`
	Project string `json:"project_id,omitempty"`
}

func BusinessScope(tenant, project string) Scope {
	if project != "" {
		return Scope{"project", tenant, project}
	}
	return Scope{"tenant", tenant, ""}
}

type Target struct {
	Kind string `json:"kind"`
	ID   string `json:"id,omitempty"`
}
type Recipient struct {
	Account           string `json:"account_id"`
	Policy            string `json:"policy"`
	Relationship      string `json:"relationship"`
	TenantMembership  string `json:"tenant_membership_id,omitempty"`
	ProjectMembership string `json:"project_membership_id,omitempty"`
}

func Personal(ids ...string) []Recipient {
	out := []Recipient{}
	for _, id := range ids {
		if id != "" {
			out = append(out, Recipient{Account: id, Policy: "personal", Relationship: "affected_account"})
		}
	}
	return out
}
func Scoped(ids ...string) []Recipient {
	out := Personal(ids...)
	for i := range out {
		out[i].Policy = "scope"
		out[i].Relationship = "manager"
	}
	return out
}

type Event struct {
	Module        string          `json:"module"`
	Key           string          `json:"key"`
	Type          string          `json:"type"`
	Scope         Scope           `json:"scope"`
	Actor         string          `json:"actor,omitempty"`
	Target        Target          `json:"target"`
	Outcome       string          `json:"outcome"`
	Reason        string          `json:"reason_code,omitempty"`
	Recipients    []Recipient     `json:"recipients"`
	OccurredAt    time.Time       `json:"-"`
	TaskReference *taskrecord.Ref `json:"task_reference,omitempty"`
}

var ErrConflict = errors.New("notification source key conflicts with an existing fact")
var code = regexp.MustCompile(`^[a-zA-Z0-9_.:-]{1,120}$`)
var targets = []string{"security", "signup_invitation", "tenant_invitation", "workspaces", "project_access", "monitoring_import", "monitoring_job", "derived_job", "derived_channel", "spatial_version", "spatial_access", "display", "file_job", "audit_export"}

func (e *Event) normalize() error {
	if _, ok := Catalog[e.Type]; !ok {
		return fmt.Errorf("unknown notification event %q", e.Type)
	}
	if !code.MatchString(e.Module) || len(e.Key) == 0 || len(e.Key) > 300 || !code.MatchString(e.Outcome) || e.Reason != "" && !code.MatchString(e.Reason) {
		return errors.New("invalid notification fact")
	}
	if !slices.Contains(targets, e.Target.Kind) || len(e.Target.ID) > 120 {
		return errors.New("invalid notification target")
	}
	if e.Target.ID != "" {
		if _, err := uuid.Parse(e.Target.ID); err != nil {
			return errors.New("invalid target ID")
		}
	}
	switch e.Scope.Kind {
	case "account", "platform":
		if e.Scope.Tenant != "" || e.Scope.Project != "" {
			return errors.New("invalid personal scope")
		}
	case "tenant", "project":
		if _, err := uuid.Parse(e.Scope.Tenant); err != nil {
			return err
		}
		if e.Scope.Kind == "project" {
			if _, err := uuid.Parse(e.Scope.Project); err != nil {
				return err
			}
		} else if e.Scope.Project != "" {
			return errors.New("invalid tenant scope")
		}
	default:
		return errors.New("invalid scope")
	}
	if e.Actor != "" {
		if _, err := uuid.Parse(e.Actor); err != nil {
			return err
		}
	}
	if e.TaskReference != nil {
		r := e.TaskReference
		valid := false
		for _, d := range taskrecord.Catalog() {
			if d.Module == r.Module && d.Kind == r.Kind {
				valid = true
			}
		}
		if !valid {
			return errors.New("invalid task reference")
		}
		if _, err := uuid.Parse(r.RootID); err != nil {
			return err
		}
		if _, err := taskrecord.Number(r.Generation, 1); err != nil {
			return err
		}
		if r.TaskID != "" {
			return errors.New("notification source references cannot assert a public ID")
		}
	}
	seen := map[string]Recipient{}
	for _, r := range e.Recipients {
		id, err := uuid.Parse(r.Account)
		if err != nil {
			return err
		}
		r.Account = id.String()
		if !slices.Contains([]string{"personal", "scope"}, r.Policy) || !code.MatchString(r.Relationship) {
			return errors.New("invalid recipient")
		}
		if prior, ok := seen[r.Account]; ok && prior.Policy != r.Policy {
			return errors.New("conflicting recipient policies")
		}
		seen[r.Account] = r
	}
	e.Recipients = make([]Recipient, 0, len(seen))
	for _, r := range seen {
		e.Recipients = append(e.Recipients, r)
	}
	slices.SortFunc(e.Recipients, func(a, b Recipient) int { return strings.Compare(a.Account, b.Account) })
	return nil
}
func Null(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// AppendTx must be called after business state and audit writes, before commit.
// It never suppresses failures; an unsupported schema also aborts the business.
func AppendTx(ctx context.Context, tx pgx.Tx, e Event) (string, error) {
	if err := e.normalize(); err != nil {
		return "", err
	}
	var version int
	if err := tx.QueryRow(ctx, `SELECT version FROM notification_protocol WHERE id=1`).Scan(&version); err != nil {
		return "", err
	}
	if version != 1 && version != 2 {
		return "", errors.New("notification protocol version mismatch")
	}
	// Replaying a pre-upgrade source key retains its exact v1 serialization.
	// A changed original payload still fails the fingerprint comparison below.
	if e.TaskReference != nil {
		var existingSchema int
		err := tx.QueryRow(ctx, `SELECT schema_version FROM notification_events WHERE source_module=$1 AND source_key=$2`, e.Module, e.Key).Scan(&existingSchema)
		if err != nil && err != pgx.ErrNoRows {
			return "", err
		}
		if err == nil && existingSchema == 1 {
			e.TaskReference = nil
		}
	}
	schema := 1
	if e.TaskReference != nil {
		schema = 2
		if version < 2 {
			return "", errors.New("notification v2 unavailable")
		}
	}
	raw, _ := json.Marshal(e)
	sum := sha256.Sum256(raw)
	fingerprint := hex.EncodeToString(sum[:])
	var id, existing string
	var at any
	if !e.OccurredAt.IsZero() {
		at = e.OccurredAt
	}
	err := tx.QueryRow(ctx, `INSERT INTO notification_events(source_module,source_key,event_type,schema_version,scope,tenant_id,project_id,actor_id,target_kind,target_id,outcome,reason_code,occurred_at,fingerprint,recipient_count,task_reference) VALUES($1,$2,$3,$15,$4,$5,$6,$7,$8,$9,$10,$11,COALESCE($12::timestamptz,clock_timestamp()),$13,$14,$16) ON CONFLICT(source_module,source_key) DO UPDATE SET source_key=EXCLUDED.source_key RETURNING id::text,fingerprint`, e.Module, e.Key, e.Type, e.Scope.Kind, Null(e.Scope.Tenant), Null(e.Scope.Project), Null(e.Actor), e.Target.Kind, e.Target.ID, e.Outcome, e.Reason, at, fingerprint, len(e.Recipients), schema, e.TaskReference).Scan(&id, &existing)
	if err != nil {
		return "", err
	}
	if fingerprint != existing {
		return "", ErrConflict
	}

	if len(e.Recipients) > 0 {
		accounts, policies, relationships, tenants, projects := []string{}, []string{}, []string{}, []string{}, []string{}
		for _, r := range e.Recipients {
			accounts = append(accounts, r.Account)
			policies = append(policies, r.Policy)
			relationships = append(relationships, r.Relationship)
			tenants = append(tenants, r.TenantMembership)
			projects = append(projects, r.ProjectMembership)
		}
		_, err = tx.Exec(ctx, `INSERT INTO notification_recipients(event_id,account_id,policy,relationship,tenant_membership_id,project_membership_id)
 SELECT $1,r.account,r.policy,r.relationship,COALESCE(NULLIF(r.tm,'')::uuid,tm.id),COALESCE(NULLIF(r.pm,'')::uuid,pm.id)
 FROM unnest($2::uuid[],$3::text[],$4::text[],$5::text[],$6::text[]) AS r(account,policy,relationship,tm,pm)
 LEFT JOIN LATERAL (SELECT id FROM tenant_memberships WHERE tenant_id=$7 AND account_id=r.account ORDER BY joined_at DESC,id DESC LIMIT 1) tm ON true
 LEFT JOIN LATERAL (SELECT id FROM project_memberships WHERE tenant_id=$7 AND project_id=$8 AND account_id=r.account ORDER BY joined_at DESC,id DESC LIMIT 1) pm ON true
 ON CONFLICT DO NOTHING`, id, accounts, policies, relationships, tenants, projects, Null(e.Scope.Tenant), Null(e.Scope.Project))
		if err != nil {
			return "", err
		}
	}

	auditScope := ar.Scope{Kind: e.Scope.Kind, Tenant: e.Scope.Tenant, Project: e.Scope.Project}
	if auditScope.Kind == "account" {
		auditScope.Kind = "platform"
	}
	if err = ar.NotificationTx(ctx, tx, id, e.Module, auditScope); err != nil {
		return "", err
	}
	return id, nil
}
func EmitTx(ctx context.Context, tx pgx.Tx, e Event) error {
	_, err := AppendTx(ctx, tx, e)
	return err
}

// ConditionTx records a domain-confirmed failure or recovery. Generation belongs
// in key; pause/cancel/reconfigure must never be reported as recovery.
func ConditionTx(ctx context.Context, tx pgx.Tx, key string, failed bool, e Event) error {
	var active bool
	var cycle int64
	err := tx.QueryRow(ctx, `SELECT active,cycle FROM notification_conditions WHERE source_module=$1 AND condition_key=$2 FOR UPDATE`, e.Module, key).Scan(&active, &cycle)
	if err == pgx.ErrNoRows {
		if !failed {
			return nil
		}
		tag, err := tx.Exec(ctx, `INSERT INTO notification_conditions(source_module,condition_key) VALUES($1,$2) ON CONFLICT DO NOTHING`, e.Module, key)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ConditionTx(ctx, tx, key, failed, e)
		}
		cycle = 1
	} else if err != nil {
		return err
	} else if active == failed {
		if failed {
			_, err = tx.Exec(ctx, `UPDATE notification_conditions SET occurrences=occurrences+1,last_at=clock_timestamp() WHERE source_module=$1 AND condition_key=$2`, e.Module, key)
		}
		return err
	} else {
		if failed {
			cycle++
		}
		_, err = tx.Exec(ctx, `UPDATE notification_conditions SET active=$3,cycle=$4,occurrences=CASE WHEN $3 THEN 1 ELSE occurrences END,last_at=clock_timestamp(),first_at=CASE WHEN $3 THEN clock_timestamp() ELSE first_at END,recovered_at=CASE WHEN $3 THEN NULL ELSE clock_timestamp() END WHERE source_module=$1 AND condition_key=$2`, e.Module, key, failed, cycle)
		if err != nil {
			return err
		}
	}
	e.Key = fmt.Sprintf("%s:%d:%t", key, cycle, failed)
	if failed {
		if e.Outcome == "" {
			e.Outcome = "failed"
		}
	} else {
		e.Outcome = "recovered"
	}
	return EmitTx(ctx, tx, e)
}

// TargetContext is a safe metadata projection from a domain-owned batch reader.
// It never contains capabilities, raw results or credential-bearing URLs.
type TargetContext struct {
	Allowed, Available  bool
	Name, State, Reason string
}
