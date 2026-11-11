package access

import (
	"context"
	"encoding/base64"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Scope is applied before name lookup/search, including the legacy cursor lookup.
// This endpoint intentionally excludes project events and project governance.
const platformAuditScope = `e.project_id IS NULL AND e.event_type<>'tenant.project.governance' AND NOT (e.tenant_id IS NOT NULL AND e.object_type IN ('engineering_definition','engineering_facility','engineering_component','engineering_point','engineering_channel'))`

const auditNames = `WITH named AS (
 SELECT e.*,
 COALESCE(NULLIF(e.details#>>'{actor,display_name}',''),e.details#>>'{actor,username}','') AS actor_snapshot,
 COALESCE(NULLIF(actor.display_name,''),actor.username,'') AS actor_current,
 COALESCE(NULLIF(e.details#>>'{actor,username}',''),actor.username,'') AS actor_username,
 CASE
 WHEN e.object_type='engineering_definition' THEN COALESCE(NULLIF(e.details#>>'{after,name}',''),NULLIF(e.details#>>'{after,code}',''),e.details#>>'{after,definition,code}',e.details#>>'{before,code}','')
 WHEN e.object_type IN ('role','tenant_role') THEN COALESCE(NULLIF(e.details#>>'{after,role,name}',''),NULLIF(e.details#>>'{after,name}',''),NULLIF(e.details#>>'{before,role,name}',''),e.details#>>'{before,name}','')
 WHEN e.object_type='tenant' THEN COALESCE(NULLIF(e.details#>>'{after,tenant,name}',''),NULLIF(e.details#>>'{after,name}',''),NULLIF(e.details#>>'{after,tenant_name}',''),e.details#>>'{before,name}','')
 WHEN e.object_type IN ('account','tenant_member') THEN COALESCE(NULLIF(e.details#>>'{after,account,display_name}',''),NULLIF(e.details#>>'{after,display_name}',''),NULLIF(e.details#>>'{after,account,username}',''),NULLIF(e.details#>>'{after,username}',''),NULLIF(e.details#>>'{before,account,display_name}',''),NULLIF(e.details#>>'{before,display_name}',''),NULLIF(e.details#>>'{before,account,username}',''),e.details#>>'{before,username}','')
 ELSE '' END AS target_snapshot,
 CASE WHEN e.object_type IN ('role','tenant_role') THEN COALESCE(r.name,'')
 WHEN e.object_type='tenant' THEN COALESCE(t.name,'')
 WHEN e.object_type='tenant_member' THEN COALESCE(NULLIF(member_account.display_name,''),member_account.username,'')
 WHEN e.object_type IN ('account','session','password_reset') THEN COALESCE(NULLIF(target.display_name,''),target.username,'')
 ELSE '' END AS target_current,
 CASE WHEN e.object_type='tenant_member' THEN COALESCE(e.details#>>'{after,account,username}',e.details#>>'{after,username}',e.details#>>'{before,account,username}',e.details#>>'{before,username}',member_account.username,'')
 WHEN e.object_type IN ('account','session','password_reset') THEN COALESCE(e.details#>>'{after,username}',e.details#>>'{before,username}',target.username,'') ELSE '' END AS target_username,
 COALESCE(t.name,'') AS tenant_name,
 CASE WHEN e.actor_type IN ('maintenance','release') OR e.event_type IN ('catalog_synced','tenant.authorization.preset_synced') THEN 'system'
 WHEN e.event_type IN ('login','logout','logout_all','session_revoked','reauthentication','password_changed','password_updated','password_reset') THEN 'security'
 ELSE 'management' END AS category
 FROM account_security_events e
 LEFT JOIN accounts actor ON actor.id=e.actor_id
 LEFT JOIN accounts target ON target.id=e.account_id
 LEFT JOIN roles r ON e.object_type IN ('role','tenant_role') AND r.id::text=e.object_id
 LEFT JOIN tenants t ON t.id=e.tenant_id
 LEFT JOIN tenant_memberships member ON e.object_type='tenant_member' AND member.id::text=e.object_id AND member.tenant_id=e.tenant_id
 LEFT JOIN accounts member_account ON member_account.id=member.account_id
 WHERE ` + platformAuditScope + ` AND ($7 OR e.tenant_id IS NULL)
 AND ($1::timestamptz IS NULL OR (e.created_at,e.id)<($1,$2))
 AND ($3='' OR e.object_id=$3 OR e.account_id::text=$3)
 AND ($4='' OR e.object_type=$4)
 AND ($5::timestamptz IS NULL OR e.created_at>=$5)
 AND ($6::timestamptz IS NULL OR e.created_at<=$6)
 AND ($8='' OR e.tenant_id::text=$8) AND ($9='' OR e.event_type=$9)
)
SELECT id::text,event_type,actor_type,COALESCE(actor_id::text,''),COALESCE(account_id::text,''),object_type,object_id,reason,request_id,created_at,details,COALESCE(tenant_id::text,''),actor_scope,
 category,outcome,actor_snapshot,actor_current,actor_username,target_snapshot,target_current,target_username,tenant_name,source_ip,user_agent
FROM named
WHERE ($10='' OR concat_ws(' ',reason,event_type,actor_snapshot,actor_current,actor_username,target_snapshot,target_current,target_username,tenant_name) ILIKE '%'||$10||'%')
AND ($11='' OR category=$11) AND ($12='' OR outcome=$12)
ORDER BY named.created_at DESC,named.id DESC LIMIT $13`

func auditCursor(ctx context.Context, tx pgx.Tx, raw string, canTenant bool) (*time.Time, int64, error) {
	if raw == "" {
		return nil, 0, nil
	}
	invalid := Validation("cursor", "分页位置无效，请回到第一页。")
	if id, err := strconv.ParseInt(raw, 10, 64); err == nil && id > 0 {
		// Old clients can finish an ID-cursor traversal in the new chronological order.
		var created time.Time
		if err = tx.QueryRow(ctx, `SELECT e.created_at FROM account_security_events e WHERE `+platformAuditScope+` AND e.id=$1 AND ($2 OR e.tenant_id IS NULL)`, id, canTenant).Scan(&created); err != nil {
			if err == pgx.ErrNoRows {
				return nil, 0, invalid
			}
			return nil, 0, err
		}
		return &created, id, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, 0, invalid
	}
	parts := strings.Split(string(decoded), "|")
	if len(parts) != 2 {
		return nil, 0, invalid
	}
	created, err := time.Parse(time.RFC3339Nano, parts[0])
	id, idErr := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || idErr != nil || id < 1 {
		return nil, 0, invalid
	}
	return &created, id, nil
}

func (s *Service) AuditEvents(ctx context.Context, a Actor, f Filter) (Page[AuditEvent], error) {
	v := Page[AuditEvent]{Items: []AuditEvent{}}
	err := s.read(ctx, a, OpAuditRead, func(tx pgx.Tx, _ string) error {
		limit, err := f.validate()
		if err != nil {
			return err
		}
		if !slices.Contains([]string{"", "management", "security", "system"}, f.Category) {
			return Validation("category", "记录分类无效。")
		}
		if !slices.Contains([]string{"", "success", "failure", "rate_limited"}, f.Outcome) {
			return Validation("outcome", "操作结果无效。")
		}
		canTenant, err := HasTx(ctx, tx, a.ID, TenantRead)
		if err != nil {
			return err
		}
		if f.TenantID != "" {
			if !canTenant {
				return ErrDenied
			}
			if !ValidID(f.TenantID) {
				return Validation("tenant_id", "租户 ID 无效。")
			}
		}
		created, id, err := auditCursor(ctx, tx, f.Cursor, canTenant)
		if err != nil {
			return err
		}
		var from, to *time.Time
		for _, part := range []struct {
			raw  string
			dest **time.Time
		}{{f.From, &from}, {f.To, &to}} {
			if part.raw != "" {
				value, err := time.Parse(time.RFC3339, part.raw)
				if err != nil {
					return Validation("time", "时间需为 RFC3339 格式。")
				}
				*part.dest = &value
			}
		}
		if from != nil && to != nil && from.After(*to) {
			return Validation("time", "起始时间不得晚于结束时间。")
		}
		rows, err := tx.Query(ctx, auditNames, created, id, strings.TrimSpace(f.ObjectID), f.ObjectType, from, to, canTenant, f.TenantID, f.Event, strings.TrimSpace(f.Query), f.Category, f.Outcome, limit+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var event AuditEvent
			display := &AuditDisplay{PermissionNames: map[string]string{}, RoleNames: map[string]string{}}
			var actorSnapshot, actorCurrent, targetSnapshot, targetCurrent string
			if err = rows.Scan(&event.ID, &event.EventType, &event.ActorType, &event.ActorID, &event.AccountID, &event.ObjectType, &event.ObjectID, &event.Reason, &event.RequestID, &event.CreatedAt, &event.Details, &event.TenantID, &event.ActorScope,
				&display.Category, &display.Outcome, &actorSnapshot, &actorCurrent, &display.ActorUsername, &targetSnapshot, &targetCurrent, &display.TargetUsername, &display.TenantName, &display.SourceIP, &display.UserAgent); err != nil {
				return err
			}
			display.ActorName, display.ActorNameSource = auditName(actorSnapshot, actorCurrent)
			display.TargetName, display.TargetNameSource = auditName(targetSnapshot, targetCurrent)
			event.Display = display
			v.Items = append(v.Items, event)
		}
		if err = rows.Err(); err != nil {
			return err
		}
		rows.Close()
		if len(v.Items) > limit {
			v.Items = v.Items[:limit]
			last := v.Items[limit-1]
			v.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(last.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + last.ID))
		}
		return auditReferences(ctx, tx, v.Items)
	})
	return v, err
}

func auditName(snapshot, current string) (string, string) {
	if snapshot != "" {
		return snapshot, "snapshot"
	}
	if current != "" {
		return current, "current"
	}
	return "", "unavailable"
}

// Resolve only references contained in this page's authorized events, in one query.
func auditReferences(ctx context.Context, tx pgx.Tx, events []AuditEvent) error {
	stringsByEvent := make([]map[string]bool, len(events))
	ids := map[string]bool{}
	var visit func(any, map[string]bool)
	visit = func(value any, values map[string]bool) {
		switch v := value.(type) {
		case string:
			values[v] = true
			if ValidID(v) {
				ids[v] = true
			}
		case []any:
			for _, item := range v {
				visit(item, values)
			}
		case map[string]any:
			for _, item := range v {
				visit(item, values)
			}
		}
	}
	for i, event := range events {
		stringsByEvent[i] = map[string]bool{}
		visit(event.Details, stringsByEvent[i])
		for _, permission := range Catalog() {
			if stringsByEvent[i][string(permission.Code)] {
				event.Display.PermissionNames[string(permission.Code)] = permission.Name
			}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	keys := make([]string, 0, len(ids))
	for id := range ids {
		keys = append(keys, id)
	}
	rows, err := tx.Query(ctx, `SELECT id::text,name FROM roles WHERE id=ANY($1::uuid[])`, keys)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, name string
		if err = rows.Scan(&id, &name); err != nil {
			return err
		}
		for i, event := range events {
			if stringsByEvent[i][id] {
				event.Display.RoleNames[id] = name
			}
		}
	}
	return rows.Err()
}
