package spatial

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const UploadPartSize int64 = 8 * 1024 * 1024
const MaxSourceSize int64 = 256 * 1024 * 1024

var shaPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func formatFor(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".png":
		return "png"
	case ".jpg", ".jpeg":
		return "jpeg"
	case ".pdf":
		return "pdf"
	case ".glb":
		return "glb"
	case ".ifc":
		return "ifc"
	case ".geojson", ".json":
		return "geojson"
	}
	return ""
}
func mediaType(format string) string {
	switch format {
	case "png":
		return "image/png"
	case "jpeg":
		return "image/jpeg"
	case "pdf":
		return "application/pdf"
	case "glb":
		return "model/gltf-binary"
	case "geojson":
		return "application/geo+json"
	}
	return "application/octet-stream"
}
func validKindFormat(kind, format string) bool {
	return kind == "layout" && (format == "png" || format == "jpeg" || format == "pdf") || kind == "model" && (format == "glb" || format == "ifc") || kind == "geolayer" && format == "geojson"
}
func uploadTx(ctx context.Context, tx pgx.Tx, tenant, id string) (Upload, error) {
	out := Upload{}
	if !access.ValidID(id) {
		return out, notFound()
	}
	e := readJSON(ctx, tx, &out, `SELECT to_jsonb(u)||jsonb_build_object('revision',revision::text) FROM spatial_uploads u WHERE tenant_id=$1 AND id=$2`, tenant, id)
	out.PartSize = UploadPartSize
	return out, e
}
func (s *Service) StartUpload(ctx context.Context, a access.Actor, scope Scope, in UploadInput) (Result, error) {
	out := Result{}
	e := s.transact(ctx, a, scope, "uploads.create", true, func(tx pgx.Tx, auth k.Authorization) error {
		if e := checkMeta(&in.Meta); e != nil {
			return e
		}
		var e error
		var replay bool
		out, replay, e = receipt(ctx, tx, auth, in.Meta, "uploads.create", in)
		if e != nil || replay {
			return e
		}
		if s.blobs == nil {
			return unavailable()
		}
		asset, v, e := s.readableTx(ctx, tx, auth, in.VersionID, false)
		if e != nil {
			return e
		}
		if e = draftable(asset, v); e != nil {
			return e
		}
		if e = k.Expect(in.Revision, v.Revision); e != nil {
			return e
		}
		if v.SourceBlobID != "" {
			return k.Problem("spatial_source_sealed", "来源已封存，更换来源请新建版本草稿。", 409)
		}
		var active bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM spatial_uploads WHERE version_id=$1 AND state='uploading')`, in.VersionID).Scan(&active); e != nil {
			return e
		}
		if active {
			return k.Problem("spatial_upload_exists", "已有未完成上传，请继续同一原件或先取消。", 409)
		}
		format := formatFor(in.Filename)
		if !validKindFormat(asset.Kind, format) || in.Size < 1 || in.Size > MaxSourceSize || !shaPattern.MatchString(in.SHA256) || filepath.Base(in.Filename) != in.Filename || len(in.Filename) > 200 || strings.ContainsAny(in.Filename, "\r\n\\") {
			return invalid("/file", "文件格式、名称、大小或 SHA-256 无效；来源最大 256 MiB。")
		}
		e = tx.QueryRow(ctx, `INSERT INTO spatial_uploads(tenant_id,version_id,actor_id,filename,size,sha256) VALUES($1,$2,$3,$4,$5,$6) RETURNING id::text,revision::text`, scope.Tenant, in.VersionID, a.ID, in.Filename, in.Size, in.SHA256).Scan(&out.ID, &out.Revision)
		if e != nil {
			return e
		}
		out.VersionID = in.VersionID
		out.Changed = true
		return complete(ctx, tx, auth, in.Meta, "uploads.create", in, out)
	})
	return out, e
}
func (s *Service) Upload(ctx context.Context, a access.Actor, scope Scope, id string) (Upload, error) {
	out := Upload{}
	e := s.transact(ctx, a, scope, "uploads.detail", false, func(tx pgx.Tx, auth k.Authorization) error {
		var e error
		out, e = uploadTx(ctx, tx, scope.Tenant, id)
		return e
	})
	return out, e
}
func (s *Service) activeUploadTx(ctx context.Context, tx pgx.Tx, auth k.Authorization, id string) (Upload, Version, error) {
	u, e := uploadTx(ctx, tx, auth.Tenant, id)
	if e != nil {
		return u, Version{}, e
	}
	asset, v, e := s.readableTx(ctx, tx, auth, u.VersionID, false)
	if e != nil {
		return u, v, e
	}
	if e = draftable(asset, v); e != nil {
		return u, v, e
	}
	if u.State != "uploading" {
		return u, v, k.Problem("upload_closed", "上传已封存或取消。", 409)
	}
	var expired bool
	e = tx.QueryRow(ctx, `SELECT expires_at<clock_timestamp() FROM spatial_uploads WHERE id=$1`, id).Scan(&expired)
	if e != nil {
		return u, v, e
	}
	if expired {
		return u, v, k.Problem("upload_expired", "上传已过期，请取消后重新开始。", 409)
	}
	return u, v, nil
}
func (s *Service) UploadPart(ctx context.Context, a access.Actor, scope Scope, id string, part int, hash string, body io.Reader) (Result, error) {
	out := Result{}
	u := Upload{}
	e := s.transact(ctx, a, scope, "uploads.part", false, func(tx pgx.Tx, auth k.Authorization) error {
		var e error
		u, _, e = s.activeUploadTx(ctx, tx, auth, id)
		return e
	})
	if e != nil {
		return out, e
	}
	if s.blobs == nil {
		return out, unavailable()
	}
	parts := (u.Size + UploadPartSize - 1) / UploadPartSize
	if part < 1 || int64(part) > parts || !shaPattern.MatchString(hash) {
		return out, invalid("/part", "分片序号或校验值无效。")
	}
	expected := UploadPartSize
	if int64(part) == parts {
		expected = u.Size - (parts-1)*UploadPartSize
	}
	data, e := io.ReadAll(io.LimitReader(body, expected+1))
	if e != nil {
		return out, e
	}
	sum := sha256.Sum256(data)
	if int64(len(data)) != expected || hex.EncodeToString(sum[:]) != hash {
		return out, invalid("/part", "分片大小或 SHA-256 不匹配。")
	}
	key := fmt.Sprintf("spatial/%s/temporary/%s/%s", scope.Tenant, id, uuid.NewString())
	if e = s.blobs.Put(ctx, key, bytes.NewReader(data), int64(len(data)), "application/octet-stream"); e != nil {
		return out, unavailable()
	}
	attached := false
	defer func() {
		if !attached {
			_ = s.blobs.Delete(context.WithoutCancel(ctx), key)
		}
	}()
	e = s.transact(ctx, a, scope, "uploads.part", true, func(tx pgx.Tx, auth k.Authorization) error {
		current, _, e := s.activeUploadTx(ctx, tx, auth, id)
		if e != nil {
			return e
		}
		var old string
		e = tx.QueryRow(ctx, `SELECT sha256 FROM spatial_upload_parts WHERE upload_id=$1 AND part=$2`, id, part).Scan(&old)
		if e == nil {
			if old != hash {
				return k.Problem("upload_part_conflict", "该分片已封存为不同内容。", 409)
			}
			out = Result{ID: id, Revision: current.Revision, Replayed: true}
			return nil
		}
		if e != pgx.ErrNoRows {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO spatial_upload_parts(tenant_id,upload_id,part,object_key,size,sha256) VALUES($1,$2,$3,$4,$5,$6)`, scope.Tenant, id, part, key, expected, hash); e != nil {
			return e
		}
		out.ID = id
		out.Changed = true
		return tx.QueryRow(ctx, `UPDATE spatial_uploads SET revision=revision+1 WHERE id=$1 RETURNING revision::text`, id).Scan(&out.Revision)
	})
	attached = e == nil && out.Changed
	return out, e
}

type uploadPart struct {
	Key, Hash string
	Size      int64
	Part      int
}

func (s *Service) CompleteUpload(ctx context.Context, a access.Actor, scope Scope, id string, in RevisionInput) (Result, error) {
	out := Result{}
	if e := checkMeta(&in.Meta); e != nil {
		return out, e
	}
	input := commandInput(id, in)
	u := Upload{}
	parts := []uploadPart{}
	replayed := false
	e := s.transact(ctx, a, scope, "uploads.complete", false, func(tx pgx.Tx, auth k.Authorization) error {
		var e error
		out, replayed, e = receipt(ctx, tx, auth, in.Meta, "uploads.complete", input)
		if e != nil || replayed {
			return e
		}
		u, _, e = s.activeUploadTx(ctx, tx, auth, id)
		if e != nil {
			return e
		}
		if e = k.Expect(in.Revision, u.Revision); e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT object_key,sha256,size,part FROM spatial_upload_parts WHERE upload_id=$1 ORDER BY part`, id)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var p uploadPart
			if e = rows.Scan(&p.Key, &p.Hash, &p.Size, &p.Part); e != nil {
				return e
			}
			parts = append(parts, p)
		}
		if int64(len(parts)) != (u.Size+UploadPartSize-1)/UploadPartSize {
			return k.Problem("upload_incomplete", "分片尚未上传完整。", 409)
		}
		return rows.Err()
	})
	if e != nil || replayed {
		return out, e
	}
	if s.blobs == nil {
		return out, unavailable()
	}
	temp, e := os.CreateTemp(s.temp, "spatial-upload-*")
	if e != nil {
		return out, unavailable()
	}
	defer os.Remove(temp.Name())
	defer temp.Close()
	full := sha256.New()
	var size int64
	for i, p := range parts {
		if p.Part != i+1 {
			return out, invalid("/parts", "上传分片不连续。")
		}
		r, e := s.blobs.Get(ctx, p.Key)
		if e != nil {
			return out, unavailable()
		}
		h := sha256.New()
		n, copyErr := io.Copy(io.MultiWriter(temp, full, h), io.LimitReader(r, p.Size+1))
		r.Close()
		if copyErr != nil || n != p.Size || hex.EncodeToString(h.Sum(nil)) != p.Hash {
			return out, invalid("/parts", "上传分片内容校验失败。")
		}
		size += n
	}
	if size != u.Size || hex.EncodeToString(full.Sum(nil)) != u.SHA256 {
		return out, invalid("/sha256", "完整文件大小或 SHA-256 不匹配。")
	}
	if _, e = temp.Seek(0, 0); e != nil {
		return out, e
	}
	blobID := uuid.NewString()
	key := fmt.Sprintf("spatial/%s/sources/%s", scope.Tenant, blobID)
	format := formatFor(u.Filename)
	if e = s.blobs.Put(ctx, key, temp, size, mediaType(format)); e != nil {
		return out, unavailable()
	}
	attached := false
	defer func() {
		if !attached {
			_ = s.blobs.Delete(context.WithoutCancel(ctx), key)
		}
	}()
	e = s.transact(ctx, a, scope, "uploads.complete", true, func(tx pgx.Tx, auth k.Authorization) error {
		var e error
		var replay bool
		out, replay, e = receipt(ctx, tx, auth, in.Meta, "uploads.complete", input)
		if e != nil || replay {
			return e
		}
		current, v, e := s.activeUploadTx(ctx, tx, auth, id)
		if e != nil {
			return e
		}
		if e = k.Expect(u.Revision, current.Revision); e != nil {
			return e
		}
		if v.SourceBlobID != "" {
			return k.Problem("spatial_source_sealed", "版本来源已封存。", 409)
		}
		_, e = tx.Exec(ctx, `INSERT INTO spatial_blobs(id,tenant_id,object_key,sha256,size,media_type,name) VALUES($1,$2,$3,$4,$5,$6,$7)`, blobID, scope.Tenant, key, u.SHA256, u.Size, mediaType(format), u.Filename)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `UPDATE spatial_asset_versions SET source_blob_id=$2,format=$3,revision=revision+1 WHERE id=$1`, u.VersionID, blobID, format)
		if e != nil {
			return e
		}
		out.ID = id
		out.VersionID = u.VersionID
		out.Changed = true
		e = tx.QueryRow(ctx, `UPDATE spatial_uploads SET state='sealed',source_blob_id=$2,revision=revision+1 WHERE id=$1 RETURNING revision::text`, id, blobID).Scan(&out.Revision)
		if e != nil {
			return e
		}
		return complete(ctx, tx, auth, in.Meta, "uploads.complete", input, out)
	})
	attached = e == nil && !out.Replayed
	if attached {
		for _, p := range parts {
			_ = s.blobs.Delete(context.WithoutCancel(ctx), p.Key)
		}
	}
	return out, e
}
func (s *Service) CancelUpload(ctx context.Context, a access.Actor, scope Scope, id string, in RevisionInput) (Result, error) {
	out := Result{}
	keys := []string{}
	e := s.transact(ctx, a, scope, "uploads.cancel", true, func(tx pgx.Tx, auth k.Authorization) error {
		if e := checkMeta(&in.Meta); e != nil {
			return e
		}
		input := commandInput(id, in)
		var e error
		var replay bool
		out, replay, e = receipt(ctx, tx, auth, in.Meta, "uploads.cancel", input)
		if e != nil || replay {
			return e
		}
		u, e := uploadTx(ctx, tx, scope.Tenant, id)
		if e != nil {
			return e
		}
		if e = k.Expect(in.Revision, u.Revision); e != nil {
			return e
		}
		if u.State != "uploading" {
			return k.Problem("upload_closed", "上传已封存或取消。", 409)
		}
		rows, e := tx.Query(ctx, `SELECT object_key FROM spatial_upload_parts WHERE upload_id=$1`, id)
		if e != nil {
			return e
		}
		for rows.Next() {
			var key string
			if e = rows.Scan(&key); e != nil {
				rows.Close()
				return e
			}
			keys = append(keys, key)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		out.ID = id
		out.Changed = true
		e = tx.QueryRow(ctx, `UPDATE spatial_uploads SET state='canceled',revision=revision+1 WHERE id=$1 RETURNING revision::text`, id).Scan(&out.Revision)
		if e != nil {
			return e
		}
		return complete(ctx, tx, auth, in.Meta, "uploads.cancel", input, out)
	})
	if e == nil && s.blobs != nil {
		for _, key := range keys {
			_ = s.blobs.Delete(context.WithoutCancel(ctx), key)
		}
	}
	return out, e
}
