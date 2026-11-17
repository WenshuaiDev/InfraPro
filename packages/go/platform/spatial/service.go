package spatial

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/jackc/pgx/v5"
	"strings"
)

type Service struct {
	basemaps     []Basemap
	access       *access.Service
	objects      ObjectReader
	monitoring   MonitoringReader
	artifacts    ArtifactReader
	blobs        BlobStore
	converter    Converter
	temp         string
	maxFileBytes int64
}

func New(a *access.Service, objects ObjectReader) *Service {
	return &Service{access: a, objects: objects, maxFileBytes: MaxSourceSize}
}
func (s *Service) WithMaxFileBytes(n int64) *Service { s.maxFileBytes = n; return s }
func (s *Service) WithStorage(b BlobStore) *Service  { s.blobs = b; return s }
func (s *Service) WithConverter(c Converter, temp string) *Service {
	s.converter = c
	s.temp = temp
	return s
}
func (s *Service) WithReaders(m MonitoringReader, a ArtifactReader) *Service {
	s.monitoring = m
	s.artifacts = a
	return s
}
func (s *Service) transact(ctx context.Context, a access.Actor, scope Scope, action string, write bool, fn func(pgx.Tx, k.Authorization) error) error {
	if scope.Entry != "tenant" && scope.Entry != "project" {
		return notFound()
	}
	return k.Transact(ctx, s.access, a, scope, access.SpatialOperation(scope.Entry, action), k.Locks{TenantWrite: write, Governance: write}, fn)
}
func notFound() error                    { return k.NotFound("spatial") }
func invalid(path, message string) error { return k.Invalid(path, "invalid", message) }
func unavailable() error {
	return k.Problem("spatial_unavailable", "空间文件或处理服务暂不可用。", 503)
}
func restricted(e error) bool {
	var ke *k.Error
	var ae *access.Error
	return errors.Is(e, access.ErrDenied) || errors.As(e, &ke) && (ke.Status == 403 || ke.Status == 404) || errors.As(e, &ae) && (ae.Status == 403 || ae.Status == 404)
}
func scopeKey(scope Scope) string {
	if scope.Project != "" {
		return scope.Project
	}
	return "tenant"
}
func readJSON(ctx context.Context, tx pgx.Tx, out any, query string, args ...any) error {
	var raw []byte
	e := tx.QueryRow(ctx, query, args...).Scan(&raw)
	if e == pgx.ErrNoRows {
		return notFound()
	}
	if e != nil {
		return e
	}
	return json.Unmarshal(raw, out)
}

const assetJSON = `to_jsonb(a)||jsonb_build_object('revision',a.revision::text,'facility_id',COALESCE(a.facility_id::text,''),'current_version_id',COALESCE(a.current_version_id::text,''))`
const versionJSON = `to_jsonb(v)||jsonb_build_object('revision',v.revision::text,'base_version_id',COALESCE(v.base_version_id::text,''))`

func assetTx(ctx context.Context, tx pgx.Tx, tenant, id string) (Asset, error) {
	out := Asset{}
	if !access.ValidID(id) {
		return out, notFound()
	}
	e := readJSON(ctx, tx, &out, `SELECT `+assetJSON+` FROM spatial_assets a WHERE tenant_id=$1 AND id=$2`, tenant, id)
	return out, e
}
func versionTx(ctx context.Context, tx pgx.Tx, tenant, id string) (Version, error) {
	out := Version{}
	if !access.ValidID(id) {
		return out, notFound()
	}
	e := readJSON(ctx, tx, &out, `SELECT `+versionJSON+` FROM spatial_asset_versions v WHERE tenant_id=$1 AND id=$2 AND deleted_at IS NULL`, tenant, id)
	if e == nil {
		e = tx.QueryRow(ctx, `SELECT COALESCE(source_blob_id::text,'') FROM spatial_asset_versions WHERE id=$1`, id).Scan(&out.SourceBlobID)
	}
	return out, e
}
func (s *Service) readableTx(ctx context.Context, tx pgx.Tx, auth k.Authorization, id string, source bool) (Asset, Version, error) {
	v, e := versionTx(ctx, tx, auth.Tenant, id)
	if e != nil {
		return Asset{}, v, e
	}
	asset, e := assetTx(ctx, tx, auth.Tenant, v.AssetID)
	if e != nil {
		return asset, v, e
	}
	if v.State == "withdrawn" {
		return asset, v, notFound()
	}
	if auth.Entry == "project" {
		var download bool
		e = tx.QueryRow(ctx, `SELECT g.source_download FROM project_spatial_grants g LEFT JOIN project_facility_links l ON l.id=g.facility_link_id AND l.tenant_id=g.tenant_id AND l.project_id=g.project_id WHERE g.tenant_id=$1 AND g.project_id=$2 AND g.version_id=$3 AND g.ended_at IS NULL AND ($4::uuid IS NULL OR (g.facility_id=$4 AND l.status='active'))`, auth.Tenant, auth.Project, id, k.Nullable(asset.FacilityID)).Scan(&download)
		if e == pgx.ErrNoRows || v.State != "published" {
			return asset, v, notFound()
		}
		if e != nil {
			return asset, v, e
		}
		v.CanDownloadSource = download && auth.Has(access.ProjectSpatialSourceDownload)
	} else {
		v.CanDownloadSource = auth.Has(access.TenantSpatialSourceDownload)
	}
	if source && !v.CanDownloadSource {
		return asset, v, notFound()
	}
	if !v.ContentAvailable {
		return asset, v, k.Problem("spatial_content_unavailable", "该固定版本的内容校验失败或文件缺失，请联系维护人员修复。", 503)
	}
	return asset, v, nil
}
func draftable(a Asset, v Version) error {
	if a.Status != "normal" {
		return k.Problem("spatial_retired", "资料已退役，请先恢复。", 409)
	}
	if v.State != "draft" {
		return k.Problem("spatial_immutable", "已发布版本不可修改，请复制为新草稿。", 409)
	}
	return nil
}
func (s *Service) objectTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, ref ObjectRef) (Object, error) {
	if s.objects == nil {
		return Object{}, unavailable()
	}
	return s.objects.ObjectTx(ctx, tx, a, scope, ref)
}
func (s *Service) sameFacility(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope, ref ObjectRef, facility string, newUse bool) (Object, error) {
	o, e := s.objectTx(ctx, tx, a, scope, ref)
	return checkObjectFacility(o, e, facility, newUse)
}

// Only use inside one authorization-locked transaction. BIM geometry fragments
// share business objects, but permissions must never be cached between requests.
func (s *Service) objectReaderTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope) func(ObjectRef) (Object, error) {
	type result struct {
		object Object
		err    error
	}
	objects := map[ObjectRef]result{}
	return func(ref ObjectRef) (Object, error) {
		if cached, ok := objects[ref]; ok {
			return cached.object, cached.err
		}
		o, err := s.objectTx(ctx, tx, a, scope, ref)
		objects[ref] = result{o, err}
		return o, err
	}
}

func (s *Service) sameFacilityReaderTx(ctx context.Context, tx pgx.Tx, a access.Actor, scope Scope) func(ObjectRef, string, bool) (Object, error) {
	read := s.objectReaderTx(ctx, tx, a, scope)
	return func(ref ObjectRef, facility string, newUse bool) (Object, error) {
		o, err := read(ref)
		return checkObjectFacility(o, err, facility, newUse)
	}
}

func checkObjectFacility(o Object, e error, facility string, newUse bool) (Object, error) {
	if e != nil {
		return o, e
	}
	if o.FacilityID != facility {
		return o, invalid("/object", "目标必须与资料属于同一设施。")
	}
	if newUse && o.Retired {
		return o, k.Problem("resource_retired", "目标或祖先已退役，不能新增挂载。", 409)
	}
	return o, nil
}
func receipt(ctx context.Context, tx pgx.Tx, auth k.Authorization, m Meta, action string, input any) (Result, bool, error) {
	out := Result{}
	var op, hash, membership string
	e := tx.QueryRow(ctx, `SELECT operation,payload_hash,membership_key,result FROM spatial_receipts WHERE tenant_id=$1 AND scope_key=$2 AND actor_id=$3 AND request_id=$4`, auth.Tenant, scopeKey(auth.Scope), auth.Account, m.RequestID).Scan(&op, &hash, &membership, &out)
	if e == pgx.ErrNoRows {
		return out, false, auditBeforeTx(ctx, tx, auth, action, input)
	}
	if e != nil {
		return out, false, e
	}
	if membership != auth.TenantMembership+":"+auth.ProjectMembership {
		return out, false, notFound()
	}
	if op != action || hash != k.Hash(input) {
		return out, false, k.Problem("request_conflict", "相同请求 ID 已用于不同内容。", 409)
	}
	out.Replayed = true
	return out, true, nil
}
func complete(ctx context.Context, tx pgx.Tx, auth k.Authorization, m Meta, action string, input any, result Result) error {
	_, e := tx.Exec(ctx, `INSERT INTO spatial_receipts(tenant_id,scope_key,actor_id,request_id,membership_key,operation,payload_hash,result) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, auth.Tenant, scopeKey(auth.Scope), auth.Account, m.RequestID, auth.TenantMembership+":"+auth.ProjectMembership, action, k.Hash(input), k.JSON(result))
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO spatial_changes(tenant_id,project_id,actor_id,operation,target_id,request_id,reason,details) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, auth.Tenant, k.Nullable(auth.Project), auth.Account, action, result.ID, m.RequestID, m.Reason, k.JSON(result))
	if e != nil {
		return e
	}
	kind := auditKind(action)
	var before any
	var raw string
	if e = tx.QueryRow(ctx, `SELECT COALESCE(NULLIF(current_setting('infrapro.spatial_audit_before',true),''),'null')`).Scan(&raw); e != nil {
		return e
	}
	if e = json.Unmarshal([]byte(raw), &before); e != nil {
		return e
	}
	after, e := auditSnapshotTx(ctx, tx, auth.Scope, kind, result.ID)
	if e != nil {
		return e
	}
	links := []ar.Link{}
	scope := ar.Scope{Kind: auth.Entry, Tenant: auth.Tenant, Project: auth.Project}
	version := result.VersionID
	if kind == "spatial_version" && version == "" {
		version = result.ID
	}
	if version != "" {
		links = append(links, ar.Link{Kind: "business", Module: "spatial", TargetKind: "spatial_version", TargetID: version, Relation: "subject", Scope: scope})
		var asset string
		e = tx.QueryRow(ctx, `SELECT asset_id::text FROM spatial_asset_versions WHERE tenant_id=$1 AND id=$2`, auth.Tenant, version).Scan(&asset)
		if e != nil && e != pgx.ErrNoRows {
			return e
		}
		if e == nil {
			links = append(links, ar.Link{Kind: "business", Module: "spatial", TargetKind: "spatial_asset", TargetID: asset, Relation: "subject", Scope: scope})
		}
	}
	if e = taskrecord.ObserveAuditCommandTx(ctx, tx); e != nil {
		return e
	}
	_, e = ar.AppendLegacyTx(ctx, tx, ar.LegacyInput{Event: "spatial." + action, ObjectType: kind, ObjectID: result.ID, Before: before, After: after, ActorID: auth.Account, ActorType: "account", Tenant: auth.Tenant, Project: auth.Project, Entry: auth.Entry, RequestID: m.RequestID, Reason: m.Reason, Links: links})
	return e
}
func checkMeta(m *Meta) error               { return m.Normalize() }
func commandInput(id string, input any) any { return []any{id, input} }
func normalizeFilter(f *Filter) error {
	if f.OwnerID != "" && !access.ValidID(f.OwnerID) {
		return invalid("/owner_id", "对象编号无效。")
	}
	f.Query = k.Text(f.Query)
	if f.Limit == 0 {
		f.Limit = 50
	}
	if f.Limit < 1 || f.Limit > 100 || len(f.Query) > 600 {
		return invalid("/filter", "分页大小须为 1–100，搜索最多 200 字符。")
	}
	if f.FacilityID != "" && !access.ValidID(f.FacilityID) {
		return invalid("/facility_id", "设施 ID 无效。")
	}
	if len(f.BBox) != 0 && len(f.BBox) != 4 {
		return invalid("/bbox", "范围需要西、南、东、北四个坐标。")
	}
	if len(f.BBox) == 4 {
		if e := validateBBox(f.BBox); e != nil {
			return e
		}
	}
	return nil
}
func cursorContext(auth k.Authorization, f Filter, extra any) string {
	f.Cursor = ""
	f.Limit = 0
	return k.Hash([]any{auth.Scope, auth.Account, auth.Revision, auth.ResourceAccessRevision, auth.TenantMembership, auth.ProjectMembership, f, extra})
}
func bumpVersion(ctx context.Context, tx pgx.Tx, id string) (string, error) {
	var rev string
	e := tx.QueryRow(ctx, `UPDATE spatial_asset_versions SET revision=revision+1 WHERE id=$1 RETURNING revision::text`, id).Scan(&rev)
	return rev, e
}
func bumpAsset(ctx context.Context, tx pgx.Tx, id string) (string, error) {
	var rev string
	e := tx.QueryRow(ctx, `UPDATE spatial_assets SET revision=revision+1 WHERE id=$1 RETURNING revision::text`, id).Scan(&rev)
	return rev, e
}
func cleanAction(s string) string { return strings.TrimSuffix(s, "-preview") }
func idError(id string) error {
	if !access.ValidID(id) {
		return invalid("/id", fmt.Sprintf("引用 ID 格式无效。"))
	}
	return nil
}
