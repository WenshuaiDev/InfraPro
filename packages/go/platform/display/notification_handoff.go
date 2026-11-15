package display

import (
	"context"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"slices"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

// appendNotificationTx only writes display-owned metadata. Package workers can
// execute the package branch without any accounts/access/inbox table privilege.
func appendNotificationTx(ctx context.Context, tx pgx.Tx, event int64, scope Scope, instance, kind string, details any) error {
	if instance == "" {
		return nil
	}
	if kind == "instance" {
		var flags struct {
			Notify bool `json:"notify_instance"`
		}
		if e := json.Unmarshal(jsonBytes(details), &flags); e != nil {
			return e
		}
		if !flags.Notify {
			return nil
		}
	}
	var taskRef *taskrecord.Ref
	category, target, outcome := "management", instance, kind
	recipients := []string{}
	snapshots := map[string][]string{}
	if kind == "package.ready" || kind == "package.failed" || kind == "package.canceled" {
		category = "package"
		var data map[string]json.RawMessage
		if e := json.Unmarshal(jsonBytes(details), &data); e != nil {
			return e
		}
		var job, pack string
		_ = json.Unmarshal(data["job_id"], &job)
		_ = json.Unmarshal(data["package_id"], &pack)
		var account, requester, generation string
		e := tx.QueryRow(ctx, `SELECT j.id::text,u.actor_id::text,j.task_requested_by::text,j.execution_generation::text FROM display_package_jobs j JOIN display_uploads u ON u.id=j.upload_id LEFT JOIN display_packages p ON p.job_id=j.id WHERE u.tenant_id=$1 AND u.project_id=$2 AND u.instance_id=$3 AND (j.id::text=$4 OR p.id::text=$5)`, scope.Tenant, scope.Project, instance, job, pack).Scan(&target, &account, &requester, &generation)
		if e != nil {
			return e
		}
		recipients = append(recipients, account)
		if requester != account {
			recipients = append(recipients, requester)
		}
		taskRef = &taskrecord.Ref{SourceRef: taskrecord.SourceRef{Module: "display", Kind: "job", RootID: target}, Generation: generation}
		outcome = strings.TrimPrefix(kind, "package.")
	} else {
		if !slices.Contains([]string{"publish", "rollback", "withdraw", "instance", "grant", "terminal.revoked", "package.missing", "package.recovered"}, kind) {
			return nil
		}
		var e error
		recipients, e = access.NotificationRecipientsTx(ctx, tx, scope.Tenant, scope.Project, displayManagerCodes()...)
		if e != nil {
			return e
		}
		for _, account := range recipients {
			var tenantMember, projectMember string
			if e = tx.QueryRow(ctx, `SELECT tenant_membership_id::text,id::text FROM project_memberships WHERE tenant_id=$1 AND project_id=$2 AND account_id=$3 AND status='active'`, scope.Tenant, scope.Project, account).Scan(&tenantMember, &projectMember); e != nil {
				return e
			}
			snapshots[account] = []string{tenantMember, projectMember}
		}
		if strings.HasPrefix(kind, "package.") {
			category = "condition"
			var data struct {
				Package string `json:"package_id"`
			}
			if e = json.Unmarshal(jsonBytes(details), &data); e != nil {
				return e
			}
			if !access.ValidID(data.Package) {
				return invalid("package", "持久故障缺少所属包。")
			}
			target = data.Package
			outcome = strings.TrimPrefix(kind, "package.")
		}
	}
	_, e := tx.Exec(ctx, `INSERT INTO display_notification_handoffs(event_id,tenant_id,project_id,instance_id,kind,target_id,outcome,recipients,membership_snapshots,task_reference) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(event_id) DO NOTHING`, event, scope.Tenant, scope.Project, instance, category, target, outcome, recipients, jsonBytes(snapshots), taskRef)
	return e
}

// PackageAvailabilityTx emits only an actual transition of a currently released
// package. It does not turn per-request errors or telemetry into notifications.
func PackageAvailabilityTx(ctx context.Context, tx pgx.Tx, id string, missing bool) error {
	if _, e := access.Lock(ctx, tx, false); e != nil {
		return e
	}
	before, after, kind := "missing", "ready", "package.recovered"
	if missing {
		before, after, kind = "ready", "missing", "package.missing"
	}
	var scope Scope
	var instance string
	e := tx.QueryRow(ctx, `UPDATE display_packages SET state=$3 WHERE id=$1 AND state=$2 RETURNING tenant_id::text,project_id::text,instance_id::text`, id, before, after).Scan(&scope.Tenant, &scope.Project, &instance)
	if e == pgx.ErrNoRows {
		return nil
	}
	if e != nil {
		return e
	}
	var published bool
	e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM display_instances i JOIN display_releases r ON r.id=i.current_release_id WHERE i.id=$1 AND r.package_id=$2 AND NOT r.withdrawn)`, instance, id).Scan(&published)
	if e != nil || !published {
		return e
	}
	return eventTx(ctx, tx, scope, instance, "", kind, map[string]string{"package_id": id})
}

type NotificationHandoff struct {
	TaskReference                    *taskrecord.Ref
	ID, Event, Kind, Target, Outcome string
	Scope                            Scope
	Instance                         string
	Recipients                       []string
	OccurredAt                       time.Time
	MembershipSnapshots              map[string][]string
}

func PendingNotificationsTx(ctx context.Context, tx pgx.Tx) ([]NotificationHandoff, error) {
	rows, e := tx.Query(ctx, `SELECT h.id::text,h.event_id::text,h.kind,h.target_id::text,h.outcome,h.tenant_id::text,h.project_id::text,h.instance_id::text,h.recipients,h.occurred_at,h.membership_snapshots,h.task_reference FROM display_notification_handoffs h JOIN display_events e ON e.id=h.event_id AND e.tenant_id=h.tenant_id AND e.project_id=h.project_id AND e.instance_id=h.instance_id WHERE h.translated_at IS NULL AND h.attempts<8 AND h.available_at<=clock_timestamp() ORDER BY h.occurred_at,h.id LIMIT 100 FOR UPDATE OF h SKIP LOCKED`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []NotificationHandoff{}
	for rows.Next() {
		var h NotificationHandoff
		if e = rows.Scan(&h.ID, &h.Event, &h.Kind, &h.Target, &h.Outcome, &h.Scope.Tenant, &h.Scope.Project, &h.Instance, &h.Recipients, &h.OccurredAt, &h.MembershipSnapshots, &h.TaskReference); e != nil {
			return nil, e
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
func ValidateNotificationTx(ctx context.Context, tx pgx.Tx, h NotificationHandoff) error {
	var valid bool
	if h.Kind == "package" {
		// A retry can already have changed the job's current state. Validate the
		// immutable terminal fact and ownership, not that later mutable state.
		e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM display_package_jobs j JOIN display_uploads u ON u.id=j.upload_id JOIN display_events e ON e.id=$6::bigint AND e.tenant_id=u.tenant_id AND e.project_id=u.project_id AND e.instance_id=u.instance_id LEFT JOIN display_packages p ON p.job_id=j.id WHERE j.id=$1 AND u.tenant_id=$2 AND u.project_id=$3 AND u.instance_id=$4 AND u.actor_id=ANY($5::uuid[]) AND e.kind='package.'||$7 AND (e.details->>'job_id'=j.id::text OR e.details->>'package_id'=p.id::text))`, h.Target, h.Scope.Tenant, h.Scope.Project, h.Instance, h.Recipients, h.Event, h.Outcome).Scan(&valid)
		if e != nil {
			return e
		}
		if valid {
			var expected []string
			if h.TaskReference == nil {
				if e = tx.QueryRow(ctx, `SELECT ARRAY[u.actor_id::text] FROM display_package_jobs j JOIN display_uploads u ON u.id=j.upload_id WHERE j.id=$1`, h.Target).Scan(&expected); e != nil {
					return e
				}
			} else {
				ref := h.TaskReference
				if ref.Module != "display" || ref.Kind != "job" || ref.RootID != h.Target {
					return problem("handoff_invalid", "任务引用与来源不符。", 409)
				}
				if _, e = taskrecord.Number(ref.Generation, 1); e != nil {
					return e
				}
				e = tx.QueryRow(ctx, `SELECT ARRAY[ snapshot->>'originator_account_id',snapshot->>'requested_by'] FROM display_task_handoffs WHERE job_id=$1 AND execution_generation=$2 AND snapshot->>'state'=CASE $3 WHEN 'ready' THEN 'succeeded' WHEN 'canceled' THEN 'cancelled' ELSE $3 END AND fingerprint=md5(snapshot::text) ORDER BY task_revision DESC LIMIT 1`, h.Target, ref.Generation, h.Outcome).Scan(&expected)
				if e == pgx.ErrNoRows {
					return problem("handoff_invalid", "缺少固定代次的来源事实。", 409)
				}
				if e != nil {
					return e
				}
			}
			actual := slices.Clone(h.Recipients)
			slices.Sort(actual)
			slices.Sort(expected)
			expected = slices.Compact(expected)
			valid = slices.Equal(actual, expected)
		}
	} else {
		e := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM display_instances i JOIN display_events e ON e.instance_id=i.id AND e.tenant_id=i.tenant_id AND e.project_id=i.project_id WHERE i.id=$1 AND i.tenant_id=$2 AND i.project_id=$3 AND e.id=$4::bigint AND (($5='management' AND e.kind=$6) OR ($5='condition' AND e.kind='package.'||$6 AND e.details->>'package_id'=$7)))`, h.Instance, h.Scope.Tenant, h.Scope.Project, h.Event, h.Kind, h.Outcome, h.Target).Scan(&valid)
		if e != nil {
			return e
		}
	}
	if !valid {
		return problem("handoff_invalid", "大屏通知交接记录无效。", 409)
	}
	return nil
}
func CompleteNotificationTx(ctx context.Context, tx pgx.Tx, id string) error {
	_, e := tx.Exec(ctx, `UPDATE display_notification_handoffs SET translated_at=clock_timestamp(),error_code='' WHERE id=$1 AND translated_at IS NULL`, id)
	return e
}

func FailNotificationTx(ctx context.Context, tx pgx.Tx, id string) error {
	_, e := tx.Exec(ctx, `UPDATE display_notification_handoffs SET attempts=attempts+1,available_at=clock_timestamp()+least(300,power(2,attempts+1))::integer*interval '1 second',error_code='translation_failed' WHERE id=$1 AND translated_at IS NULL`, id)
	return e
}
func ReplayNotificationTx(ctx context.Context, tx pgx.Tx, id string) error {
	_, e := tx.Exec(ctx, `UPDATE display_notification_handoffs SET attempts=0,available_at=clock_timestamp(),error_code='' WHERE id=$1 AND translated_at IS NULL AND error_code<>''`, id)
	return e
}
