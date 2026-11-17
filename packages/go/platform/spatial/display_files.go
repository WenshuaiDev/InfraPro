package spatial

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"slices"
	"strconv"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
)

func (s *Service) displayBasemaps() displayread.Materialized {
	items := []displayread.Basemap{{ID: "blank", Name: "空白背景", State: "available", Attribution: ""}, {ID: "local", Name: "本地示意底图", State: "available", Attribution: "InfraPro 合成示例 · 非测绘资料", MinZoom: 0, MaxZoom: 18}}
	for _, b := range s.basemaps {
		if b.ID == "local" || b.ID == "blank" {
			continue
		}
		items = append(items, displayread.Basemap{ID: b.ID, Name: b.Name, State: "unavailable", Reason: "isolated_runtime_external_basemap", Attribution: b.Attribution, MinZoom: b.MinZoom, MaxZoom: b.MaxZoom})
	}
	return displayread.Materialized{Single: map[string]any{"items": items}}
}
func (s *Service) displayTile(q displayread.TileRequest) (displayread.Materialized, error) {
	if q.BasemapID != "local" {
		return displayread.Materialized{}, displayread.Problem("capability_unavailable", "隔离运行环境只提供 local 示意瓦片。", 409)
	}
	data, e := LocalTile(fmt.Sprintf("%d-%d-%d.png", q.Z, q.X, q.Y))
	if e != nil {
		return displayread.Materialized{}, e
	}
	sum := sha256.Sum256(data)
	return displayread.Materialized{Single: displayread.FileChunk{ID: "local", Base64: base64.StdEncoding.EncodeToString(data), MediaType: "image/png", SHA256: hex.EncodeToString(sum[:]), Size: strconv.Itoa(len(data)), Offset: "0", Length: strconv.Itoa(len(data))}}, nil
}
func (s *Service) displayFileTx(ctx context.Context, tx pgx.Tx, p displayread.Plan, auth k.Authorization, q displayread.SpatialFileRequest) (displayread.Materialized, error) {
	out := displayread.Materialized{}
	if !slices.Contains(p.Scopes["target"].IDs("version"), q.ID) || q.FileID == "source" {
		return out, displayread.Hidden()
	}
	f, key, e := s.fileTx(ctx, tx, auth, q.ID, q.FileID, false)
	if e != nil {
		return out, e
	}
	if s.blobs == nil {
		return out, displayread.Problem("dependency_unavailable", "空间展示文件存储不可用。", 503)
	}
	length := q.Length
	if length == 0 {
		length = f.Size - q.Offset
		if length > displayread.MaxFileBytes {
			length = displayread.MaxFileBytes
		}
	}
	if q.Offset < 0 || length < 0 || q.Offset > f.Size || length > f.Size-q.Offset {
		return out, displayread.Invalid("/offset", "分块范围超出展示文件。")
	}
	r, e := s.blobs.Get(ctx, key)
	if e != nil {
		return out, displayread.Problem("dependency_unavailable", "固定空间展示文件不可用。", 503)
	}
	defer r.Close()
	// Verify the complete immutable artifact while retaining only this bounded
	// chunk. Storage keys and original source bytes never enter the read result.
	hash := sha256.New()
	reader := io.TeeReader(io.LimitReader(r, f.Size+1), hash)
	if _, e = io.CopyN(io.Discard, reader, q.Offset); e != nil {
		return out, displayread.Problem("dependency_unavailable", "展示文件完整性校验失败。", 503)
	}
	data := make([]byte, length)
	if _, e = io.ReadFull(reader, data); e != nil {
		return out, displayread.Problem("dependency_unavailable", "展示文件完整性校验失败。", 503)
	}
	rest, e := io.Copy(io.Discard, reader)
	if e != nil || q.Offset+length+rest != f.Size || hex.EncodeToString(hash.Sum(nil)) != f.SHA256 {
		return out, displayread.Problem("dependency_unavailable", "展示文件完整性校验失败。", 503)
	}
	out.Single = displayread.FileChunk{ID: f.ID, Base64: base64.StdEncoding.EncodeToString(data), MediaType: f.MediaType, SHA256: f.SHA256, Size: strconv.FormatInt(f.Size, 10), Offset: strconv.FormatInt(q.Offset, 10), Length: strconv.FormatInt(length, 10)}
	return out, nil
}
