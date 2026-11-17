package classification

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
)

type Service struct{ access *access.Service }

func New(a *access.Service) *Service { return &Service{access: a} }
func (s *Service) transact(ctx context.Context, a access.Actor, scope Scope, action string, write bool, fn func(pgx.Tx, k.Authorization) error) error {
	return k.Transact(ctx, s.access, a, scope, scope.Operation("classification", action), k.Locks{CatalogWrite: write, TenantWrite: write}, func(tx pgx.Tx, auth k.Authorization) error {
		lock := " FOR SHARE"
		if write {
			lock = " FOR UPDATE"
		}
		if _, e := tx.Exec(ctx, `SELECT id FROM device_classification_state WHERE id=1`+lock); e != nil {
			return e
		}
		return fn(tx, auth)
	})
}
func readDictionary(ctx context.Context, tx pgx.Tx) (Dictionary, error) {
	out := Dictionary{}
	e := tx.QueryRow(ctx, `SELECT revision::text,draft_revision::text,draft,COALESCE(current_release_id::text,'') FROM device_classification_state WHERE id=1`).Scan(&out.Revision, &out.DraftRevision, &out.Draft, &out.CurrentReleaseID)
	return out, e
}
func releaseTx(ctx context.Context, tx pgx.Tx, id string) (Release, error) {
	out := Release{}
	if id == "" {
		if e := tx.QueryRow(ctx, `SELECT COALESCE(current_release_id::text,'') FROM device_classification_state WHERE id=1`).Scan(&id); e != nil {
			return out, e
		}
	}
	if !access.ValidID(id) {
		return out, k.Problem("classification_not_published", "设备分类尚无发布版本。", 409)
	}
	e := tx.QueryRow(ctx, `SELECT id::text,number,content,content_hash,created_at,reason FROM device_category_releases WHERE id=$1`, id).Scan(&out.ID, &out.Number, &out.Content, &out.Hash, &out.CreatedAt, &out.Reason)
	if e == pgx.ErrNoRows {
		return out, k.NotFound("classification")
	}
	return out, e
}
func (s *Service) Dictionary(ctx context.Context, a access.Actor) (Dictionary, error) {
	out := Dictionary{}
	e := s.transact(ctx, a, k.Platform(), "dictionary", false, func(tx pgx.Tx, auth k.Authorization) error {
		var e error
		out, e = readDictionary(ctx, tx)
		out.CanManage = auth.Has(access.DeviceCategoryManage)
		return e
	})
	return out, e
}
func replay(ctx context.Context, tx pgx.Tx, auth k.Authorization, m Meta, action string, input any) (Result, bool, error) {
	out := Result{}
	var entry, tenant, member, op, hash string
	e := tx.QueryRow(ctx, `SELECT entry_scope,COALESCE(tenant_id::text,''),COALESCE(membership_id::text,''),action,payload_hash,result FROM device_classification_receipts WHERE actor_id=$1 AND request_id=$2`, auth.Account, m.RequestID).Scan(&entry, &tenant, &member, &op, &hash, &out)
	if e == pgx.ErrNoRows {
		return out, false, nil
	}
	if e != nil {
		return out, false, e
	}
	if entry != auth.Entry || tenant != auth.Tenant || member != auth.TenantMembership {
		return out, false, access.ErrDenied
	}
	if op != action || hash != k.Hash(input) {
		return out, false, k.Problem("request_conflict", "该请求编号已用于其他分类操作。", 409)
	}
	out.Replayed = true
	return out, true, nil
}
func receipt(ctx context.Context, tx pgx.Tx, auth k.Authorization, m Meta, action string, input any, out Result) error {
	_, e := tx.Exec(ctx, `INSERT INTO device_classification_receipts(actor_id,request_id,entry_scope,tenant_id,membership_id,action,payload_hash,result) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, auth.Account, m.RequestID, auth.Entry, k.Nullable(auth.Tenant), k.Nullable(auth.TenantMembership), action, k.Hash(input), k.JSON(out))
	return e
}
func change(ctx context.Context, tx pgx.Tx, auth k.Authorization, m Meta, action, kind, id string, before, after any, meta access.Meta) error {
	_, e := tx.Exec(ctx, `INSERT INTO device_classification_changes(entry_scope,tenant_id,actor_id,action,target_kind,target_id,before,after,request_id,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, auth.Entry, k.Nullable(auth.Tenant), auth.Account, action, kind, id, k.JSON(before), k.JSON(after), m.RequestID, m.Reason)
	if e != nil {
		return e
	}
	return k.AuditSnapshots(ctx, tx, auth, m, "classification."+action, "device_classification_"+kind, id, before, after, meta)
}
func (s *Service) SaveDraft(ctx context.Context, a access.Actor, in SaveInput, meta access.Meta) (Result, error) {
	out := Result{}
	if e := in.Meta.Normalize(); e != nil {
		return out, e
	}
	if e := in.Meta.RequireReason(); e != nil {
		return out, e
	}
	content, e := Normalize(in.Content, false)
	if e != nil {
		return out, e
	}
	in.Content = content
	e = s.transact(ctx, a, k.Platform(), "draft", true, func(tx pgx.Tx, auth k.Authorization) error {
		var e error
		var ok bool
		out, ok, e = replay(ctx, tx, auth, in.Meta, "draft", in)
		if e != nil || ok {
			return e
		}
		d, e := readDictionary(ctx, tx)
		if e != nil {
			return e
		}
		if e = k.Expect(in.Revision, d.DraftRevision); e != nil {
			return e
		}
		if d.CurrentReleaseID != "" {
			release, e := releaseTx(ctx, tx, d.CurrentReleaseID)
			if e != nil {
				return e
			}
			if in.Content.SensorRootID != release.Content.SensorRootID {
				return k.Invalid("/sensor_root_id", "stable_identity", "已发布传感器分支的稳定 ID 不能改义或替换。")
			}
			present := map[string]bool{}
			for _, n := range content.Nodes {
				present[n.ID] = true
			}
			for _, n := range release.Content.Nodes {
				if !present[n.ID] {
					return k.Invalid("/nodes", "retained_category", "已发布分类不能删除，请停用并保留身份。")
				}
			}
		}
		for _, n := range content.Nodes {
			var code string
			e := tx.QueryRow(ctx, `SELECT code FROM device_category_identities WHERE id=$1`, n.ID).Scan(&code)
			if e == nil && code != n.Code {
				return k.Invalid("/nodes/code", "immutable_code", "同一分类身份的编号不可改写或复用。")
			}
			if e != nil && e != pgx.ErrNoRows {
				return e
			}
			if e == pgx.ErrNoRows {
				if _, e = tx.Exec(ctx, `INSERT INTO device_category_identities(id,code) VALUES($1,$2)`, n.ID, n.Code); e != nil {
					return e
				}
			}
		}
		out.ID = "00000000-0000-4000-8000-000000000001"
		out.Changed = k.Hash(d.Draft) != k.Hash(content)
		if e = tx.QueryRow(ctx, `UPDATE device_classification_state SET draft=$1,draft_revision=draft_revision+1,draft_updated_at=clock_timestamp(),draft_updated_by=$2 WHERE id=1 RETURNING draft_revision::text`, k.JSON(content), auth.Account).Scan(&out.Revision); e != nil {
			return e
		}
		if e = change(ctx, tx, auth, in.Meta, "draft", "dictionary", out.ID, d.Draft, content, meta); e != nil {
			return e
		}
		return receipt(ctx, tx, auth, in.Meta, "draft", in, out)
	})
	return out, e
}
func publishPreview(ctx context.Context, tx pgx.Tx, d Dictionary, in PublishInput) (PublishPreview, Content, error) {
	out := PublishPreview{Changes: []CategoryChange{}, ExistingReferencesPreserved: true, AffectsNewGrouping: true}
	if e := k.Expect(in.Revision, d.DraftRevision); e != nil {
		return out, Content{}, e
	}
	c, e := Normalize(d.Draft, true)
	if e != nil {
		return out, c, e
	}
	before := map[string]Node{}
	if d.CurrentReleaseID != "" {
		r, e := releaseTx(ctx, tx, d.CurrentReleaseID)
		if e != nil {
			return out, c, e
		}
		for _, n := range r.Content.Nodes {
			before[n.ID] = n
		}
	}
	for _, n := range c.Nodes {
		old, exists := before[n.ID]
		if !exists || k.Hash(old) != k.Hash(n) {
			var b *Node
			if exists {
				b = &old
			}
			out.Changes = append(out.Changes, CategoryChange{ID: n.ID, Before: b, After: n})
		}
	}
	out.Hash = k.Hash([]any{d.DraftRevision, d.CurrentReleaseID, d.Revision, c})
	return out, c, nil
}
func (s *Service) PreviewPublish(ctx context.Context, a access.Actor, in PublishInput) (PublishPreview, error) {
	out := PublishPreview{}
	if e := in.Meta.Normalize(); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, k.Platform(), "publish-preview", false, func(tx pgx.Tx, auth k.Authorization) error {
		d, e := readDictionary(ctx, tx)
		if e != nil {
			return e
		}
		out, _, e = publishPreview(ctx, tx, d, in)
		return e
	})
	return out, e
}
func (s *Service) Publish(ctx context.Context, a access.Actor, in PublishInput, meta access.Meta) (Result, error) {
	out := Result{}
	if e := in.Meta.Normalize(); e != nil {
		return out, e
	}
	if e := in.Meta.RequireReason(); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, k.Platform(), "publish", true, func(tx pgx.Tx, auth k.Authorization) error {
		var e error
		var ok bool
		out, ok, e = replay(ctx, tx, auth, in.Meta, "publish", in)
		if e != nil || ok {
			return e
		}
		d, e := readDictionary(ctx, tx)
		if e != nil {
			return e
		}
		preview, c, e := publishPreview(ctx, tx, d, in)
		if e != nil {
			return e
		}
		if in.PreviewHash == "" || in.PreviewHash != preview.Hash {
			return k.Problem("preview_changed", "分类发布依据已变化，请重新预览。", 409)
		}
		var number int
		e = tx.QueryRow(ctx, `INSERT INTO device_category_releases(number,content,content_hash,created_by,reason) SELECT COALESCE(max(number),0)+1,$1,$2,$3,$4 FROM device_category_releases RETURNING id::text,number`, k.JSON(c), k.Hash(c), auth.Account, in.Reason).Scan(&out.ReleaseID, &number)
		if e != nil {
			return e
		}
		for _, n := range c.Nodes {
			if _, e = tx.Exec(ctx, `INSERT INTO device_category_release_nodes(release_id,category_id,parent_id,name,description,sort_order,selectable,status,path,name_path,sensor) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, out.ReleaseID, n.ID, k.Nullable(n.ParentID), n.Name, n.Description, n.Order, n.Selectable, n.Status, n.Path, n.NamePath, n.Sensor); e != nil {
				return e
			}
		}
		if e = tx.QueryRow(ctx, `UPDATE device_classification_state SET current_release_id=$1,revision=revision+1 WHERE id=1 RETURNING revision::text`, out.ReleaseID).Scan(&out.Revision); e != nil {
			return e
		}
		out.ID = out.ReleaseID
		out.Changed = true
		if e = change(ctx, tx, auth, in.Meta, "publish", "release", out.ID, d.CurrentReleaseID, map[string]any{"release_id": out.ID, "number": number, "content": c, "changes": preview.Changes}, meta); e != nil {
			return e
		}
		return receipt(ctx, tx, auth, in.Meta, "publish", in, out)
	})
	return out, e
}
func (s *Service) Releases(ctx context.Context, a access.Actor, scope Scope, cursor string, limit int) (Page[Release], error) {
	out := Page[Release]{Items: []Release{}}
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 500 {
		return out, k.Invalid("/limit", "limit", "每页最多 500 个版本。")
	}
	after := 0
	if cursor != "" {
		v, e := strconv.Atoi(cursor)
		if e != nil || v < 0 {
			return out, k.Invalid("/cursor", "cursor", "分页游标无效。")
		}
		after = v
	}
	e := s.transact(ctx, a, scope, "releases", false, func(tx pgx.Tx, auth k.Authorization) error {
		rows, e := tx.Query(ctx, `SELECT id::text,number,content_hash,created_at,reason FROM device_category_releases WHERE number>$1 ORDER BY number LIMIT $2`, after, limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var r Release
			if e = rows.Scan(&r.ID, &r.Number, &r.Hash, &r.CreatedAt, &r.Reason); e != nil {
				return e
			}
			r.Content = Content{DictionaryCode: DictionaryCode, Nodes: []Node{}}
			out.Items = append(out.Items, r)
		}
		if e = rows.Err(); e != nil {
			return e
		}
		if len(out.Items) > limit {
			out.Items = out.Items[:limit]
			out.NextCursor = strconv.Itoa(out.Items[limit-1].Number)
		}
		return nil
	})
	return out, e
}
func (s *Service) Categories(ctx context.Context, a access.Actor, scope Scope, release, cursor, query string, limit int) (Page[Node], error) {
	out := Page[Node]{Items: []Node{}}
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 500 || cursor != "" && !access.ValidID(cursor) {
		return out, k.Invalid("/limit", "pagination", "分类分页参数无效。")
	}
	if len([]rune(query)) > 200 {
		return out, k.Invalid("/q", "length", "搜索最多 200 字符。")
	}
	e := s.transact(ctx, a, scope, "categories", false, func(tx pgx.Tx, auth k.Authorization) error {
		r, e := releaseTx(ctx, tx, release)
		if e != nil {
			return e
		}
		out.DictionaryReleaseID = r.ID
		rows, e := tx.Query(ctx, `SELECT n.category_id::text,i.code,n.name,COALESCE(n.parent_id::text,''),n.description,n.sort_order,n.selectable,n.status,n.path,n.name_path,n.sensor FROM device_category_release_nodes n JOIN device_category_identities i ON i.id=n.category_id WHERE n.release_id=$1 AND ($2='' OR n.category_id>NULLIF($2,'')::uuid) AND ($3='' OR n.name ILIKE '%'||$3||'%' OR i.code ILIKE '%'||$3||'%') ORDER BY n.category_id LIMIT $4`, r.ID, cursor, query, limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var n Node
			if e = rows.Scan(&n.ID, &n.Code, &n.Name, &n.ParentID, &n.Description, &n.Order, &n.Selectable, &n.Status, &n.Path, &n.NamePath, &n.Sensor); e != nil {
				return e
			}
			out.Items = append(out.Items, n)
		}
		if e = rows.Err(); e != nil {
			return e
		}
		if len(out.Items) > limit {
			out.Items = out.Items[:limit]
			out.NextCursor = out.Items[limit-1].ID
		}
		return nil
	})
	return out, e
}
func decodeEffective(raw []byte) (Effective, error) {
	out := Effective{Path: []string{}, NamePath: []string{}, OverrideRevision: "0", ProductRevision: "0"}
	e := json.Unmarshal(raw, &out)
	return out, e
}
