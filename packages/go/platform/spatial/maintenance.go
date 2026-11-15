package spatial

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type IntegrityResult struct {
	VersionID string   `json:"version_id"`
	State     string   `json:"state"`
	Files     int      `json:"files"`
	Bytes     int64    `json:"bytes"`
	Available bool     `json:"available"`
	Applied   bool     `json:"applied"`
	Issues    []string `json:"issues"`
}

// VerifyContent is a host-maintenance seam, intentionally absent from HTTP. It
// checks exact sealed bytes, never regenerates or replaces a published file.
// Database and object storage must belong to the same restored environment.
// apply changes only availability, after rechecking the immutable references.
func VerifyContent(ctx context.Context, pool *pgxpool.Pool, store BlobStore, tenant string, apply bool, reason string, emit func(IntegrityResult) error) error {
	if !access.ValidID(tenant) || store == nil || emit == nil || apply && strings.TrimSpace(reason) == "" {
		return fmt.Errorf("tenant, object storage, output and an apply reason are required")
	}
	after := "00000000-0000-0000-0000-000000000000"
	for {
		var id string
		e := pool.QueryRow(ctx, `SELECT id::text FROM spatial_asset_versions WHERE tenant_id=$1 AND id>$2 ORDER BY id LIMIT 1`, tenant, after).Scan(&id)
		if e == pgx.ErrNoRows {
			return nil
		}
		if e != nil {
			return e
		}
		after = id
		var v Version
		type blob struct {
			ID, Key, Hash, Media string
			Size                 int64
		}
		files := []blob{}
		tx, e := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
		if e != nil {
			return e
		}
		v, e = versionTx(ctx, tx, tenant, id)
		if e == nil {
			var rows pgx.Rows
			rows, e = tx.Query(ctx, `SELECT x.file_id,b.object_key,b.sha256,b.media_type,b.size FROM (SELECT 'source' AS file_id,source_blob_id AS blob_id FROM spatial_asset_versions WHERE tenant_id=$1 AND id=$2 AND source_blob_id IS NOT NULL UNION ALL SELECT file_id,blob_id FROM spatial_version_files WHERE tenant_id=$1 AND version_id=$2) x JOIN spatial_blobs b ON b.tenant_id=$1 AND b.id=x.blob_id ORDER BY x.file_id`, tenant, id)
			if e == nil {
				for rows.Next() {
					var b blob
					if e = rows.Scan(&b.ID, &b.Key, &b.Hash, &b.Media, &b.Size); e != nil {
						break
					}
					files = append(files, b)
				}
				if e == nil {
					e = rows.Err()
				}
				rows.Close()
			}
		}
		_ = tx.Rollback(ctx)
		if e != nil {
			return e
		}
		report := IntegrityResult{VersionID: id, State: v.State, Available: true, Issues: []string{}}
		if v.ManifestHash != "" {
			if v.ManifestHash != k.Hash(v.Manifest) || v.Manifest.Contract != "infrapro.spatial/1" {
				report.Issues = append(report.Issues, "manifest_hash")
			}
			if len(files) != len(v.Manifest.Files)+1 {
				report.Issues = append(report.Issues, "file_inventory")
			}
			expected := map[string]File{}
			for _, f := range v.Manifest.Files {
				expected[f.ID] = f
			}
			for _, b := range files {
				if b.ID == "source" {
					if b.Hash != v.Manifest.SourceSHA256 {
						report.Issues = append(report.Issues, "source_hash")
					}
					continue
				}
				f, ok := expected[b.ID]
				if !ok || f.SHA256 != b.Hash || f.Size != b.Size || f.MediaType != b.Media {
					report.Issues = append(report.Issues, "file_reference:"+b.ID)
				}
			}
		} else if v.State != "draft" {
			report.Issues = append(report.Issues, "published_without_manifest")
		}
		transient := false
		for _, b := range files {
			report.Files++
			r, err := store.Get(ctx, b.Key)
			if err != nil {
				if !errors.Is(err, fs.ErrNotExist) {
					transient = true
				}
				report.Issues = append(report.Issues, "unreadable:"+b.ID)
				continue
			}
			h := sha256.New()
			n, err := io.Copy(h, io.LimitReader(r, b.Size+1))
			_ = r.Close()
			report.Bytes += n
			if err != nil {
				transient = true
				report.Issues = append(report.Issues, "read_interrupted:"+b.ID)
			} else if n != b.Size || hex.EncodeToString(h.Sum(nil)) != b.Hash {
				report.Issues = append(report.Issues, "integrity:"+b.ID)
			}
		}
		report.Available = len(report.Issues) == 0
		if apply && !transient {
			tx, e = pool.Begin(ctx)
			if e != nil {
				return e
			}
			_, e = access.Lock(ctx, tx, true)
			if e == nil {
				var current Version
				current, e = versionTx(ctx, tx, tenant, id)
				if e == nil && (current.Revision != v.Revision || current.ManifestHash != v.ManifestHash || current.SourceBlobID != v.SourceBlobID) {
					e = fmt.Errorf("version changed during verification; rerun")
				}
			}
			if e == nil {
				_, e = tx.Exec(ctx, `UPDATE spatial_asset_versions SET content_available=$3 WHERE tenant_id=$1 AND id=$2`, tenant, id, report.Available)
			}
			if e == nil {
				_, e = tx.Exec(ctx, `INSERT INTO spatial_changes(tenant_id,operation,target_id,reason,details) VALUES($1,'maintenance.integrity',$2,$3,$4)`, tenant, id, reason, k.JSON(report))
			}
			if e == nil {
				e = access.Audit(ctx, tx, "spatial.maintenance.integrity", "spatial_version", id, "", nil, report, access.Meta{ActorScope: "tenant", ActorType: "maintenance", TenantID: tenant, Reason: reason})
			}
			if e == nil {
				e = tx.Commit(ctx)
			} else {
				_ = tx.Rollback(ctx)
			}
			if e != nil {
				return e
			}
			report.Applied = true
		}
		if e = emit(report); e != nil {
			return e
		}
	}
}
