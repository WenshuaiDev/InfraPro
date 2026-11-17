package notifications

import (
	"context"
	"errors"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"slices"
	"strconv"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	ne "github.com/WenshuaiDev/InfraPro/packages/go/platform/notificationevent"
	"github.com/jackc/pgx/v5"
)

type Reference struct {
	TaskReference                       *taskrecord.Ref
	TenantMembership, ProjectMembership string
	EventID                             string
	Type                                string
	Scope                               ne.Scope
	Target                              ne.Target
	Outcome                             string
}
type Disclosure struct {
	TaskID, TaskGeneration string
	Allowed                bool
	ScopeName              string
	ObjectName             string
	CurrentState           string
	TargetAvailable        bool
	Reason                 string
}

// Resolver owns current business qualification. Implementations batch references;
// the inbox does not copy business authorization SQL or grant download abilities.
type Resolver interface {
	ResolveTx(context.Context, pgx.Tx, string, []Reference) (map[string]Disclosure, error)
}
type Service struct {
	access      *access.Service
	resolver    Resolver
	options     Options
	maintenance []func(context.Context) error
}

func New(a *access.Service, r Resolver) *Service {
	return &Service{access: a, resolver: r, options: DefaultOptions()}
}
func (s *Service) WithMaintenance(f ...func(context.Context) error) *Service {
	s.maintenance = f
	return s
}
func (s *Service) transaction(ctx context.Context, a access.Actor, fn func(pgx.Tx, string) error) error {
	if a.Check == nil || a.ReadSubject() != nil || !access.ValidID(a.ID) {
		return access.ErrDenied
	}
	return s.access.Transact(ctx, func(tx pgx.Tx) error {
		rev, e := access.Lock(ctx, tx, false)
		if e != nil {
			return e
		}
		if e = a.Check(ctx, tx, false); e != nil {
			return e
		}
		return fn(tx, rev)
	})
}

type Filter struct {
	Status   string `json:"status,omitempty"`
	Category string `json:"category,omitempty"`
	Tenant   string `json:"tenant_id,omitempty"`
	Project  string `json:"project_id,omitempty"`
	Scope    string `json:"scope,omitempty"`
	Cursor   string `json:"cursor,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

func (f Filter) validate() error {
	if !slices.Contains([]string{"", "all", "unread", "archived"}, f.Status) || !slices.Contains([]string{"", "security", "organization", "tasks", "updates"}, f.Category) || !slices.Contains([]string{"", "account", "platform", "tenant", "project"}, f.Scope) {
		return access.Validation("filter", "通知筛选无效。")
	}
	if f.Tenant != "" && !access.ValidID(f.Tenant) || f.Project != "" && (!access.ValidID(f.Project) || f.Tenant == "") || f.Limit < 0 || f.Limit > 100 {
		return access.Validation("filter", "通知范围或数量无效。")
	}
	if f.Cursor != "" {
		if _, e := number(f.Cursor); e != nil {
			return e
		}
	}
	return nil
}
func number(s string) (int64, error) {
	n, e := strconv.ParseInt(s, 10, 64)
	if e != nil || n < 0 {
		return 0, access.Validation("revision", "通知版本或水位无效。")
	}
	return n, nil
}

type Item struct {
	TaskID              string     `json:"task_id,omitempty"`
	TaskGeneration      string     `json:"task_generation,omitempty"`
	LegacyTaskReference bool       `json:"legacy_task_reference"`
	ID                  string     `json:"id"`
	Sequence            string     `json:"sequence"`
	Revision            string     `json:"revision"`
	Type                string     `json:"type"`
	Category            string     `json:"category"`
	Title               string     `json:"title"`
	Body                string     `json:"body"`
	Scope               ne.Scope   `json:"scope"`
	ScopeName           string     `json:"scope_name,omitempty"`
	ObjectName          string     `json:"object_name,omitempty"`
	Outcome             string     `json:"outcome"`
	ReasonCode          string     `json:"reason_code,omitempty"`
	CurrentState        string     `json:"current_state,omitempty"`
	Redacted            bool       `json:"redacted"`
	TargetAvailable     bool       `json:"target_available"`
	UnavailableReason   string     `json:"unavailable_reason,omitempty"`
	OccurredAt          time.Time  `json:"occurred_at"`
	DeliveredAt         time.Time  `json:"delivered_at"`
	ReadAt              *time.Time `json:"read_at"`
	ArchivedAt          *time.Time `json:"archived_at"`
	reference           Reference
	policy              string
}
type Page struct {
	Items                 []Item `json:"items"`
	NextCursor            string `json:"next_cursor,omitempty"`
	Watermark             string `json:"watermark"`
	Revision              string `json:"revision"`
	AuthorizationRevision string `json:"authorization_revision"`
}
type Summary struct {
	Unread                int64            `json:"unread"`
	Categories            map[string]int64 `json:"categories"`
	Revision              string           `json:"revision"`
	AuthorizationRevision string           `json:"authorization_revision"`
	Watermark             string           `json:"watermark"`
}

const itemSelect = `SELECT n.id::text,n.sequence::text,n.revision::text,e.id::text,e.event_type,e.scope,COALESCE(e.tenant_id::text,''),COALESCE(e.project_id::text,''),e.target_kind,e.target_id,e.outcome,e.reason_code,e.occurred_at,n.delivered_at,n.read_at,n.archived_at,r.policy,COALESCE(r.tenant_membership_id::text,''),COALESCE(r.project_membership_id::text,''),e.task_reference FROM account_notifications n JOIN notification_events e ON e.id=n.event_id JOIN notification_recipients r ON r.event_id=n.event_id AND r.account_id=n.account_id `

func readItem(row pgx.Row) (Item, error) {
	var v Item
	e := row.Scan(&v.ID, &v.Sequence, &v.Revision, &v.reference.EventID, &v.Type, &v.Scope.Kind, &v.Scope.Tenant, &v.Scope.Project, &v.reference.Target.Kind, &v.reference.Target.ID, &v.Outcome, &v.ReasonCode, &v.OccurredAt, &v.DeliveredAt, &v.ReadAt, &v.ArchivedAt, &v.policy, &v.reference.TenantMembership, &v.reference.ProjectMembership, &v.reference.TaskReference)
	v.LegacyTaskReference = v.reference.TaskReference == nil && slices.Contains([]string{"task.result", "spatial.result", "display.package"}, v.Type)
	v.reference.Type = v.Type
	v.reference.Scope = v.Scope
	v.reference.Outcome = v.Outcome
	return v, e
}
func (s *Service) disclose(ctx context.Context, tx pgx.Tx, account string, items []Item) ([]Item, error) {
	refs := make([]Reference, len(items))
	for i, v := range items {
		refs[i] = v.reference
	}
	if s.resolver == nil {
		return nil, errors.New("notification resolver unavailable")
	}
	resolved, e := s.resolver.ResolveTx(ctx, tx, account, refs)
	if e != nil {
		return nil, e
	}
	out := make([]Item, 0, len(items))
	for _, v := range items {
		d := resolved[v.reference.EventID]
		if !d.Allowed && v.policy != "personal" {
			continue
		}
		definition, ok := ne.Catalog[v.Type]
		if !ok {
			return nil, errors.New("unknown notification template")
		}
		v.Category = definition.Category
		v.Title = definition.Title
		v.Body = outcomeText(v.Outcome)
		v.Redacted = !d.Allowed
		if v.Redacted {
			v.Title = definition.SafeTitle
			v.ReasonCode = ""
			v.Scope = ne.Scope{Kind: "account"}
			v.UnavailableReason = "access_removed"
			if v.reference.Target.Kind == "tenant_invitation" && slices.Contains([]string{"accepted", "replaced", "revoked", "expired", "invalidated"}, d.CurrentState) {
				v.CurrentState = d.CurrentState
				v.Body = outcomeText(d.CurrentState)
				v.UnavailableReason = "invitation_" + d.CurrentState
			}
		} else {
			v.TaskID = d.TaskID
			v.TaskGeneration = d.TaskGeneration
			v.ScopeName = d.ScopeName
			v.ObjectName = d.ObjectName
			v.CurrentState = d.CurrentState
			v.TargetAvailable = d.TargetAvailable || d.TaskID != ""
			v.UnavailableReason = d.Reason
		}
		out = append(out, v)
	}
	return out, nil
}
func outcomeText(outcome string) string {
	mfa := map[string]string{
		"mfa_enabled":                    "已启用多因素认证。请妥善保存恢复码。",
		"mfa_replaced":                   "已更换验证器，旧验证器和旧恢复码已失效。",
		"mfa_disabled":                   "已关闭个人多因素认证。",
		"mfa_recovery_codes_regenerated": "已重建恢复码，旧码组已失效。",
		"mfa_reset":                      "管理员已重置多因素认证。请通过受控恢复流程重新绑定验证器。",
		"mfa_recovered":                  "多因素认证恢复完成，已绑定新的验证器。",
	}
	if v := mfa[outcome]; v != "" {
		return v
	}
	m := map[string]string{"succeeded": "处理已完成。请进入业务页面查看当前结果。", "ready": "结果已就绪。", "failed": "处理未成功，请进入业务页面查看和处理。", "needs_action": "需要你继续处理。", "cancelled": "任务已取消。", "canceled": "任务已取消。", "validated": "校验已完成，等待你确认采用。", "adopted": "数据已正式采用。", "pending": "你收到一份租户邀请，请明确确认是否加入。", "accepted": "邀请已接受。", "replaced": "邀请已替换，请查看新的邀请。", "revoked": "邀请或授权已撤销。", "expired": "有效期已结束。", "invalidated": "邀请已失效。", "recovered": "此前的故障条件已经恢复。", "missing": "已发布包暂时不可用，请检查存储。"}
	if v := m[outcome]; v != "" {
		return v
	}
	return "状态已更新，可查看当前可用的业务信息。"
}
func matches(v Item, f Filter) bool {
	if f.Status == "archived" {
		if v.ArchivedAt == nil {
			return false
		}
	} else if v.ArchivedAt != nil {
		return false
	}
	if f.Status == "unread" && v.ReadAt != nil {
		return false
	}
	return (f.Category == "" || v.Category == f.Category) && (f.Tenant == "" || v.Scope.Tenant == f.Tenant) && (f.Project == "" || v.Scope.Project == f.Project) && (f.Scope == "" || v.Scope.Kind == f.Scope)
}
func inboxState(ctx context.Context, tx pgx.Tx, account string) (string, string, error) {
	var seq, rev string
	e := tx.QueryRow(ctx, `SELECT COALESCE((SELECT sequence FROM notification_inbox_states WHERE account_id=$1),0)::text,COALESCE((SELECT revision FROM notification_inbox_states WHERE account_id=$1),0)::text`, account).Scan(&seq, &rev)
	return seq, rev, e
}

// walk bounds memory and resolves business eligibility BEFORE pagination/counting.
// No limit is applied to eligible rows until the same disclosure predicate runs.
func (s *Service) walk(ctx context.Context, tx pgx.Tx, account string, f Filter, watermark int64, visit func(Item) (bool, error)) error {
	before := watermark + 1
	if f.Cursor != "" {
		n, _ := number(f.Cursor)
		if n < before {
			before = n
		}
	}
	for {
		rows, e := tx.Query(ctx, itemSelect+` WHERE n.account_id=$1 AND n.sequence<$2 AND n.sequence<=$3 ORDER BY n.sequence DESC LIMIT 256`, account, before, watermark)
		if e != nil {
			return e
		}
		batch := []Item{}
		for rows.Next() {
			v, e := readItem(rows)
			if e != nil {
				rows.Close()
				return e
			}
			batch = append(batch, v)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if len(batch) == 0 {
			return nil
		}
		before, _ = number(batch[len(batch)-1].Sequence)
		visible, e := s.disclose(ctx, tx, account, batch)
		if e != nil {
			return e
		}
		for _, v := range visible {
			if matches(v, f) {
				stop, e := visit(v)
				if e != nil || stop {
					return e
				}
			}
		}
		if len(batch) < 256 {
			return nil
		}
	}
}
func (s *Service) List(ctx context.Context, a access.Actor, f Filter) (Page, error) {
	out := Page{Items: []Item{}}
	if e := f.validate(); e != nil {
		return out, e
	}
	if f.Limit == 0 {
		f.Limit = 30
	}
	e := s.transaction(ctx, a, func(tx pgx.Tx, rev string) error {
		out.AuthorizationRevision = rev
		var e error
		out.Watermark, out.Revision, e = inboxState(ctx, tx, a.ID)
		if e != nil {
			return e
		}
		max, _ := number(out.Watermark)
		return s.walk(ctx, tx, a.ID, f, max, func(v Item) (bool, error) {
			if len(out.Items) == f.Limit {
				out.NextCursor = out.Items[len(out.Items)-1].Sequence
				return true, nil
			}
			out.Items = append(out.Items, v)
			return false, nil
		})
	})
	return out, e
}
func (s *Service) Summary(ctx context.Context, a access.Actor) (Summary, error) {
	out := Summary{Categories: map[string]int64{"security": 0, "organization": 0, "tasks": 0, "updates": 0}}
	e := s.transaction(ctx, a, func(tx pgx.Tx, rev string) error {
		out.AuthorizationRevision = rev
		var e error
		out.Watermark, out.Revision, e = inboxState(ctx, tx, a.ID)
		if e != nil {
			return e
		}
		max, _ := number(out.Watermark)
		return s.walk(ctx, tx, a.ID, Filter{Status: "unread"}, max, func(v Item) (bool, error) { out.Unread++; out.Categories[v.Category]++; return false, nil })
	})
	return out, e
}
func (s *Service) getTx(ctx context.Context, tx pgx.Tx, account, id string) (Item, error) {
	if !access.ValidID(id) {
		return Item{}, access.ErrNotFound
	}
	v, e := readItem(tx.QueryRow(ctx, itemSelect+` WHERE n.account_id=$1 AND n.id=$2`, account, id))
	if e == pgx.ErrNoRows {
		return Item{}, access.ErrNotFound
	}
	if e != nil {
		return v, e
	}
	items, e := s.disclose(ctx, tx, account, []Item{v})
	if e != nil {
		return v, e
	}
	if len(items) == 0 {
		return Item{}, access.ErrNotFound
	}
	return items[0], nil
}
func (s *Service) Get(ctx context.Context, a access.Actor, id string) (Item, error) {
	var out Item
	e := s.transaction(ctx, a, func(tx pgx.Tx, _ string) error { var e error; out, e = s.getTx(ctx, tx, a.ID, id); return e })
	return out, e
}

type Target struct {
	Generation          string `json:"generation,omitempty"`
	LegacyTaskReference bool   `json:"legacy_task_reference"`
	Status              string `json:"status"`
	Kind                string `json:"kind,omitempty"`
	ID                  string `json:"id,omitempty"`
	Tenant              string `json:"tenant_id,omitempty"`
	Project             string `json:"project_id,omitempty"`
	Reason              string `json:"reason,omitempty"`
}

func (s *Service) Target(ctx context.Context, a access.Actor, id string) (Target, error) {
	v, e := s.Get(ctx, a, id)
	if e != nil {
		return Target{}, e
	}
	if !v.TargetAvailable {
		return Target{Status: "unavailable", Reason: v.UnavailableReason}, nil
	}
	if v.TaskID != "" {
		return Target{Status: "available", Kind: "task", ID: v.TaskID, Generation: v.TaskGeneration}, nil
	}
	return Target{Status: "available", Kind: v.reference.Target.Kind, ID: v.reference.Target.ID, Tenant: v.Scope.Tenant, Project: v.Scope.Project, LegacyTaskReference: v.LegacyTaskReference}, nil
}
func lockInbox(ctx context.Context, tx pgx.Tx, account string) error {
	if _, e := tx.Exec(ctx, `INSERT INTO notification_inbox_states(account_id) VALUES($1) ON CONFLICT DO NOTHING`, account); e != nil {
		return e
	}
	_, e := tx.Exec(ctx, `SELECT account_id FROM notification_inbox_states WHERE account_id=$1 FOR UPDATE`, account)
	return e
}

type Change struct {
	ExpectedRevision string `json:"expected_revision"`
	Read             *bool  `json:"read,omitempty"`
	Archived         *bool  `json:"archived,omitempty"`
}

func (s *Service) Change(ctx context.Context, a access.Actor, id string, in Change) (Item, error) {
	var out Item
	if _, e := number(in.ExpectedRevision); e != nil {
		return out, e
	}
	if in.Read == nil && in.Archived == nil || in.Read != nil && !*in.Read && in.Archived != nil && *in.Archived {
		return out, access.Validation("state", "请指定有效的阅读或归档状态。")
	}
	e := s.transaction(ctx, a, func(tx pgx.Tx, _ string) error {
		if e := lockInbox(ctx, tx, a.ID); e != nil {
			return e
		}
		v, e := s.getTx(ctx, tx, a.ID, id)
		if e != nil {
			return e
		}
		read, archived := v.ReadAt != nil, v.ArchivedAt != nil
		if in.Read != nil {
			read = *in.Read
			if !read {
				archived = false
			}
		}
		if in.Archived != nil {
			archived = *in.Archived
			if archived {
				read = true
			}
		}
		if read == (v.ReadAt != nil) && archived == (v.ArchivedAt != nil) {
			out = v
			return nil
		}
		if v.Revision != in.ExpectedRevision {
			return access.ErrConflict
		}
		if _, e = tx.Exec(ctx, `UPDATE account_notifications SET read_at=CASE WHEN $3 THEN COALESCE(read_at,clock_timestamp()) ELSE NULL END,archived_at=CASE WHEN $4 THEN COALESCE(archived_at,clock_timestamp()) ELSE NULL END,revision=revision+1 WHERE account_id=$1 AND id=$2`, a.ID, id, read, archived); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE notification_inbox_states SET revision=revision+1 WHERE account_id=$1`, a.ID); e != nil {
			return e
		}
		out, e = s.getTx(ctx, tx, a.ID, id)
		return e
	})
	return out, e
}

type ReadAllInput struct {
	Filter    Filter `json:"filter"`
	Watermark string `json:"watermark"`
}

func (s *Service) ReadAll(ctx context.Context, a access.Actor, in ReadAllInput) (int64, error) {
	var changed int64
	if e := in.Filter.validate(); e != nil {
		return 0, e
	}
	if in.Filter.Status == "archived" || in.Filter.Cursor != "" {
		return 0, access.Validation("filter", "全部已读仅适用于当前未归档筛选。")
	}
	max, e := number(in.Watermark)
	if e != nil {
		return 0, e
	}
	e = s.transaction(ctx, a, func(tx pgx.Tx, _ string) error {
		if e := lockInbox(ctx, tx, a.ID); e != nil {
			return e
		}
		seq, _, e := inboxState(ctx, tx, a.ID)
		if e != nil {
			return e
		}
		current, _ := number(seq)
		if max > current {
			return access.Validation("watermark", "请刷新通知后重试。")
		}
		in.Filter.Status = "unread"
		e = s.walk(ctx, tx, a.ID, in.Filter, max, func(v Item) (bool, error) {
			tag, e := tx.Exec(ctx, `UPDATE account_notifications SET read_at=clock_timestamp(),revision=revision+1 WHERE account_id=$1 AND id=$2 AND read_at IS NULL`, a.ID, v.ID)
			changed += tag.RowsAffected()
			return false, e
		})
		if e != nil {
			return e
		}
		if changed > 0 {
			_, e = tx.Exec(ctx, `UPDATE notification_inbox_states SET revision=revision+1 WHERE account_id=$1`, a.ID)
		}
		return e
	})
	return changed, e
}

type Preferences struct {
	Tasks    bool   `json:"tasks"`
	Updates  bool   `json:"updates"`
	Revision string `json:"revision"`
}

func (s *Service) Preferences(ctx context.Context, a access.Actor) (Preferences, error) {
	out := Preferences{true, true, "0"}
	e := s.transaction(ctx, a, func(tx pgx.Tx, _ string) error {
		e := tx.QueryRow(ctx, `SELECT tasks,updates,revision::text FROM notification_preferences WHERE account_id=$1`, a.ID).Scan(&out.Tasks, &out.Updates, &out.Revision)
		if e == pgx.ErrNoRows {
			return nil
		}
		return e
	})
	return out, e
}
func (s *Service) SavePreferences(ctx context.Context, a access.Actor, in Preferences) (Preferences, error) {
	revision, e := number(in.Revision)
	if e != nil {
		return in, e
	}
	e = s.transaction(ctx, a, func(tx pgx.Tx, _ string) error {
		if e := lockInbox(ctx, tx, a.ID); e != nil {
			return e
		}
		var v Preferences
		err := tx.QueryRow(ctx, `SELECT tasks,updates,revision::text FROM notification_preferences WHERE account_id=$1`, a.ID).Scan(&v.Tasks, &v.Updates, &v.Revision)
		if err != nil && err != pgx.ErrNoRows {
			return err
		}
		if err == nil && v.Tasks == in.Tasks && v.Updates == in.Updates {
			in = v
			return nil
		}
		if err == nil && v.Revision != in.Revision || err == pgx.ErrNoRows && revision != 0 {
			return access.ErrConflict
		}
		return tx.QueryRow(ctx, `INSERT INTO notification_preferences(account_id,tasks,updates) VALUES($1,$2,$3) ON CONFLICT(account_id) DO UPDATE SET tasks=EXCLUDED.tasks,updates=EXCLUDED.updates,revision=notification_preferences.revision+1 RETURNING revision::text`, a.ID, in.Tasks, in.Updates).Scan(&in.Revision)
	})
	return in, e
}
func sourceError(e error) string {
	if errors.Is(e, errTemplate) {
		return "invalid_event_template"
	}
	var ae *access.Error
	if errors.As(e, &ae) {
		return "qualification_unavailable"
	}
	return fmt.Sprintf("%s", "delivery_dependency_failed")
}
