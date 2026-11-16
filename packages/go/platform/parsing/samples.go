package parsing

import (
	"bytes"
	"context"
	"errors"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"io"
	"io/fs"
	"slices"
	"time"
)

type object struct {
	ID, Key, Hash, State string
	Size                 int64
}

func objectTx(ctx context.Context, tx pgx.Tx, id string) (object, error) {
	var o object
	e := tx.QueryRow(ctx, `SELECT id::text,object_key,COALESCE(sha256,''),state,size_bytes FROM parsing_objects WHERE id=$1`, id).Scan(&o.ID, &o.Key, &o.Hash, &o.State, &o.Size)
	return o, e
}
func (s *Service) reserveTx(ctx context.Context, tx pgx.Tx, scope Scope, size int64) (object, error) {
	var used int64
	if e := tx.QueryRow(ctx, `SELECT COALESCE(sum(size_bytes),0) FROM parsing_objects WHERE scope_key=$1 AND state<>'deleted'`, scopeKey(scope)).Scan(&used); e != nil {
		return object{}, e
	}
	if used+size > s.options.StorageBudget {
		return object{}, k.Problem("storage_budget_exceeded", "当前范围的样本与结果存储预算已用尽；历史不会自动删除。", 429)
	}
	o := object{ID: uuid.NewString(), State: "intent", Size: size}
	o.Key = "parsing/v1/" + scopeKey(scope) + "/" + o.ID
	_, e := tx.Exec(ctx, `INSERT INTO parsing_objects(id,scope_key,object_key,size_bytes) VALUES($1,$2,$3,$4)`, o.ID, scopeKey(scope), o.Key, size)
	return o, e
}
func sampleTx(ctx context.Context, tx pgx.Tx, scope Scope, id string) (Sample, error) {
	var v Sample
	var raw []byte
	e := tx.QueryRow(ctx, `SELECT to_jsonb(s)-'sha256'-'byte_length'||jsonb_build_object('sha256',COALESCE(sha256,''),'byte_length',COALESCE(byte_length,0)),object_id::text,representation_object_id::text FROM parsing_samples s WHERE scope_key=$1 AND id=$2`, scopeKey(scope), id).Scan(&raw, &v.ObjectID, &v.RepresentationObjectID)
	if e == pgx.ErrNoRows {
		return v, k.NotFound("parsing")
	}
	if e != nil {
		return v, e
	}
	e = decode(raw, &v)
	return v, e
}
func (s *Service) CreateSample(ctx context.Context, a access.Actor, scope Scope, in SampleInput) (Result, error) {
	var out Result
	if e := validateMeta(&in.Meta, false); e != nil {
		return out, e
	}
	if !slices.Contains([]string{"hex", "utf8", "binary"}, in.Representation) || !slices.Contains([]string{"synthetic", "uploaded"}, in.Source) || in.ReceivedAt.IsZero() {
		return out, k.Invalid("/sample", "invalid_sample", "请明确输入表示、来源及带时区的接收时间。")
	}
	if s.objects == nil {
		return out, k.Problem("storage_unconfigured", "样本存储尚未配置。", 503)
	}
	e := s.transact(ctx, a, scope, "samples", "create", true, func(tx pgx.Tx, auth k.Authorization) error {
		v, ok, e := replay(ctx, tx, auth, "samples.create", in.Meta, in)
		if e != nil {
			return e
		}
		if ok {
			out = v
			return nil
		}
		raw, e := s.reserveTx(ctx, tx, scope, c.MaxInput)
		if e != nil {
			return e
		}
		repr, e := s.reserveTx(ctx, tx, scope, c.MaxRequest)
		if e != nil {
			return e
		}
		e = tx.QueryRow(ctx, `INSERT INTO parsing_samples(scope_key,tenant_id,object_id,representation_object_id,representation,source,received_at,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text`, scopeKey(scope), k.Nullable(scope.Tenant), raw.ID, repr.ID, in.Representation, in.Source, in.ReceivedAt.UTC(), a.ID).Scan(&out.ID)
		if e != nil {
			return e
		}
		out.Revision = "1"
		out.State = "uploading"
		return complete(ctx, tx, auth, "samples.create", in.Meta, in, out, nil, map[string]string{"id": out.ID, "source": in.Source})
	})
	return out, e
}
func (s *Service) putImmutable(ctx context.Context, o object, b []byte) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if s.objects == nil {
		return k.Problem("storage_unconfigured", "对象存储尚未配置。", 503)
	}
	if e := s.objects.Put(ctx, o.Key, bytes.NewReader(b), int64(len(b)), "application/octet-stream"); e == nil {
		return nil
	}
	// A lost PUT response can be recovered only after verifying exact bytes.
	existing, e := s.readObject(ctx, o, c.MaxResponse)
	if e != nil || c.Digest(existing) != c.Digest(b) {
		return k.Problem("object_write_failed", "对象尚未完整保存，请用原请求重试。", 503)
	}
	return nil
}
func (s *Service) readObject(ctx context.Context, o object, max int) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if s.objects == nil {
		return nil, k.Problem("storage_unconfigured", "对象存储尚未配置。", 503)
	}
	r, e := s.objects.Get(ctx, o.Key)
	if e != nil {
		if !errors.Is(e, fs.ErrNotExist) {
			return nil, k.Problem("object_read_failed", "证据对象暂时不可读。", 503)
		}
		return nil, k.Problem("result_missing", "原始证据对象缺失，不能重新计算冒充原结果。", 404)
	}
	defer r.Close()
	b, e := io.ReadAll(io.LimitReader(r, int64(max+1)))
	if e != nil || len(b) > max {
		return nil, k.Problem("object_read_failed", "证据读取失败或超过预算。", 503)
	}
	if o.Hash != "" && (c.Digest(b) != o.Hash || int64(len(b)) != o.Size) {
		return nil, k.Problem("object_hash_mismatch", "证据摘要不符。", 409)
	}
	return b, nil
}
func (s *Service) UploadSample(ctx context.Context, a access.Actor, scope Scope, id string, m Meta, body io.Reader) (Result, error) {
	var out Result
	if e := validateMeta(&m, false); e != nil {
		return out, e
	}
	var sample Sample
	var rawObject, reprObject object
	e := s.transact(ctx, a, scope, "samples", "content", false, func(tx pgx.Tx, _ k.Authorization) error {
		var e error
		sample, e = sampleTx(ctx, tx, scope, id)
		if e != nil {
			return e
		}
		if sample.CreatedBy != a.ID {
			return access.ErrDenied
		}
		if sample.State == "expired" {
			return k.NotFound("parsing")
		}
		rawObject, e = objectTx(ctx, tx, sample.ObjectID)
		if e != nil {
			return e
		}
		reprObject, e = objectTx(ctx, tx, sample.RepresentationObjectID)
		return e
	})
	if e != nil {
		return out, e
	}
	input, e := io.ReadAll(io.LimitReader(body, c.MaxRequest+1))
	if e != nil || len(input) > c.MaxRequest {
		return out, k.Problem("limit_exceeded", "样本输入超过大小限制。", 413)
	}
	raw, e := c.DecodeRepresentation(sample.Representation, input)
	if e != nil {
		return out, k.Problem(e.Error(), "样本表示无效，请检查完整 HEX 字节对或 UTF-8。", 422)
	}
	payload := []any{id, c.Digest(input)}
	var replayed bool
	e = s.transact(ctx, a, scope, "samples", "content", true, func(tx pgx.Tx, auth k.Authorization) error {
		var e error
		out, replayed, e = replay(ctx, tx, auth, "samples.content", m, payload)
		if e != nil || replayed {
			return e
		}
		v, e := sampleTx(ctx, tx, scope, id)
		if e != nil {
			return e
		}
		if v.State != "uploading" {
			return k.Problem("sample_immutable", "已确认的样本不可覆盖。", 409)
		}
		for _, pair := range []struct {
			o object
			b []byte
		}{{rawObject, raw}, {reprObject, input}} {
			o, e := objectTx(ctx, tx, pair.o.ID)
			if e != nil {
				return e
			}
			if o.Hash != "" && o.Hash != c.Digest(pair.b) {
				return k.Problem("sample_immutable", "上传意图已固定其他字节。", 409)
			}
			if _, e = tx.Exec(ctx, `UPDATE parsing_objects SET sha256=$2,size_bytes=$3 WHERE id=$1 AND state='intent'`, o.ID, c.Digest(pair.b), len(pair.b)); e != nil {
				return e
			}
		}
		return nil
	})
	if e != nil || replayed {
		return out, e
	}
	if e = s.putImmutable(ctx, rawObject, raw); e != nil {
		return out, e
	}
	if e = s.putImmutable(ctx, reprObject, input); e != nil {
		return out, e
	}
	e = s.transact(ctx, a, scope, "samples", "content", true, func(tx pgx.Tx, auth k.Authorization) error {
		v, ok, e := replay(ctx, tx, auth, "samples.content", m, payload)
		if e != nil {
			return e
		}
		if ok {
			out = v
			return nil
		}
		sample, e = sampleTx(ctx, tx, scope, id)
		if e != nil {
			return e
		}
		if sample.State != "uploading" {
			return k.Problem("sample_immutable", "样本已经封存。", 409)
		}
		if _, e = tx.Exec(ctx, `UPDATE parsing_objects SET state='available' WHERE id=ANY($1::uuid[]) AND state='intent'`, []string{rawObject.ID, reprObject.ID}); e != nil {
			return e
		}
		out = Result{ID: id, Revision: "1", State: "uploaded"}
		return complete(ctx, tx, auth, "samples.content", m, payload, out, nil, map[string]any{"sha256": c.Digest(raw), "size_bytes": len(raw)})
	})
	return out, e
}
func (s *Service) ConfirmSample(ctx context.Context, a access.Actor, scope Scope, id string, m Meta) (Result, error) {
	var out Result
	if e := validateMeta(&m, false); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, "samples", "confirm", true, func(tx pgx.Tx, auth k.Authorization) error {
		sample, e := sampleTx(ctx, tx, scope, id)
		if e != nil {
			return e
		}
		if sample.CreatedBy != a.ID {
			return access.ErrDenied
		}
		payload := []any{id, m}
		v, ok, e := replay(ctx, tx, auth, "samples.confirm", m, payload)
		if e != nil {
			return e
		}
		if ok {
			out = v
			return nil
		}
		o, e := objectTx(ctx, tx, sample.ObjectID)
		if e != nil {
			return e
		}
		repr, e := objectTx(ctx, tx, sample.RepresentationObjectID)
		if e != nil {
			return e
		}
		if o.State != "available" || repr.State != "available" || o.Hash == "" {
			return k.Problem("upload_incomplete", "原始字节尚未完整上传。", 409)
		}
		if _, e = tx.Exec(ctx, `UPDATE parsing_samples SET state='ready',sha256=$2,byte_length=$3 WHERE id=$1 AND state='uploading'`, id, o.Hash, o.Size); e != nil {
			return e
		}
		out = Result{ID: id, Revision: "1", State: "ready"}
		return complete(ctx, tx, auth, "samples.confirm", m, payload, out, nil, map[string]any{"sha256": o.Hash, "size_bytes": o.Size})
	})
	return out, e
}
func (s *Service) Sample(ctx context.Context, a access.Actor, scope Scope, id string) (Sample, error) {
	var out Sample
	e := s.transact(ctx, a, scope, "samples", "detail", false, func(tx pgx.Tx, _ k.Authorization) error { var e error; out, e = sampleTx(ctx, tx, scope, id); return e })
	return out, e
}
func (s *Service) Samples(ctx context.Context, a access.Actor, scope Scope, f Filter) (k.Page[Sample], error) {
	out := k.Page[Sample]{Items: []Sample{}}
	if e := f.Validate(); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, "samples", "list", false, func(tx pgx.Tx, auth k.Authorization) error {
		cur, e := k.DecodeCursor(f.Cursor, k.CursorContext(auth, f, "samples"))
		if e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT to_jsonb(s)-'sha256'-'byte_length'||jsonb_build_object('sha256',COALESCE(sha256,''),'byte_length',COALESCE(byte_length,0)) FROM parsing_samples s WHERE scope_key=$1 AND state='ready' AND ($2::uuid IS NULL OR id>$2) ORDER BY id LIMIT $3`, scopeKey(scope), k.Nullable(cur.ID), f.Limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			v, e := readJSON[Sample](rows)
			if e != nil {
				return e
			}
			if len(out.Items) == f.Limit {
				out.NextCursor = k.Cursor{Scope: cur.Scope, ID: out.Items[len(out.Items)-1].ID}.Encode()
				break
			}
			out.Items = append(out.Items, v)
		}
		return rows.Err()
	})
	return out, e
}
func (s *Service) SampleContent(ctx context.Context, a access.Actor, scope Scope, id string, representation bool) ([]byte, error) {
	var o object
	authorize := func(audit bool) error {
		return s.transact(ctx, a, scope, "samples", "download", false, func(tx pgx.Tx, auth k.Authorization) error {
			v, e := sampleTx(ctx, tx, scope, id)
			if e != nil {
				return e
			}
			if v.State != "ready" {
				return k.NotFound("parsing")
			}
			objectID := v.ObjectID
			if representation {
				objectID = v.RepresentationObjectID
			}
			o, e = objectTx(ctx, tx, objectID)
			if e != nil {
				return e
			}
			if audit {
				return access.Audit(ctx, tx, "parsing.samples.read", "parsing_samples", id, "", nil, map[string]any{"sha256": v.SHA256, "size_bytes": v.ByteLength}, access.Meta{ActorID: a.ID, ActorScope: auth.Entry, TenantID: auth.Tenant, RequestID: uuid.NewString()})
			}
			return nil
		})
	}
	if e := authorize(false); e != nil {
		return nil, e
	}
	b, e := s.readObject(ctx, o, c.MaxRequest)
	if e != nil {
		return nil, e
	}
	if e = authorize(true); e != nil {
		return nil, e
	}
	return b, nil
}

// Cleanup only selects registered intents and unreferenced temporary samples.
// Objects referenced by a run or attempt are retained indefinitely.
func (s *Service) Cleanup(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if s.objects == nil {
		return nil
	}
	candidates := []object{}
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `SELECT s.id::text FROM parsing_samples s WHERE created_at<clock_timestamp()-interval '24 hours' AND state<>'expired' AND NOT EXISTS(SELECT 1 FROM parsing_runs r WHERE r.sample_id=s.id) ORDER BY created_at LIMIT 100 FOR UPDATE SKIP LOCKED`)
		if e != nil {
			return e
		}
		ids := []string{}
		for rows.Next() {
			var id string
			if e = rows.Scan(&id); e != nil {
				rows.Close()
				return e
			}
			ids = append(ids, id)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		for _, id := range ids {
			if _, e = tx.Exec(ctx, `UPDATE parsing_samples s SET state='expired' WHERE id=$1 AND NOT EXISTS(SELECT 1 FROM parsing_runs r WHERE r.sample_id=s.id)`, id); e != nil {
				return e
			}
		}
		if e != nil {
			return e
		}
		rows, e = tx.Query(ctx, `SELECT o.id::text,o.object_key FROM parsing_objects o WHERE o.created_at<clock_timestamp()-interval '24 hours' AND o.state<>'deleted' AND NOT EXISTS(SELECT 1 FROM parsing_samples s WHERE (s.object_id=o.id OR s.representation_object_id=o.id) AND s.state<>'expired') AND NOT EXISTS(SELECT 1 FROM parsing_run_executions e WHERE e.result_object_id=o.id) AND NOT EXISTS(SELECT 1 FROM parsing_run_attempts a WHERE a.response_object_id=o.id OR (a.state='running' AND a.started_at>clock_timestamp()-interval '1 hour')) ORDER BY o.created_at LIMIT 100 FOR UPDATE SKIP LOCKED`)
		if e != nil {
			return e
		}
		for rows.Next() {
			var o object
			if e = rows.Scan(&o.ID, &o.Key); e != nil {
				rows.Close()
				return e
			}
			candidates = append(candidates, o)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		for _, o := range candidates {
			if _, e = tx.Exec(ctx, `UPDATE parsing_objects SET state='delete_pending' WHERE id=$1`, o.ID); e != nil {
				return e
			}
		}
		return nil
	})
	if e != nil {
		return e
	}
	for _, o := range candidates {
		if e = s.objects.Delete(ctx, o.Key); e != nil {
			return e
		}
		if e = s.access.Transact(ctx, func(tx pgx.Tx) error {
			_, e := tx.Exec(ctx, `UPDATE parsing_objects SET state='deleted' WHERE id=$1 AND state='delete_pending'`, o.ID)
			return e
		}); e != nil {
			return e
		}
	}
	return nil
}
