package spatial

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
	"io"
	"io/fs"
	"os"
)

type Download struct {
	observer *access.TransferObserver
	File
	file                *os.File
	service             *Service
	actor               access.Actor
	scope               Scope
	version, id, action string
}

func (d *Download) Close() error {
	if d.file != nil {
		name := d.file.Name()
		d.file.Close()
		os.Remove(name)
		d.file = nil
	}
	return d.observer.Close()
}
func (s *Service) fileTx(ctx context.Context, tx pgx.Tx, auth k.Authorization, version, id string, source bool) (File, string, error) {
	out := File{}
	_, v, e := s.readableTx(ctx, tx, auth, version, source)
	if e != nil {
		return out, "", e
	}
	var key string
	if source {
		if v.SourceBlobID == "" {
			return out, "", notFound()
		}
		e = tx.QueryRow(ctx, `SELECT id::text,name,sha256,size,media_type,object_key FROM spatial_blobs WHERE tenant_id=$1 AND id=$2`, auth.Tenant, v.SourceBlobID).Scan(&out.ID, &out.Name, &out.SHA256, &out.Size, &out.MediaType, &key)
	} else {
		e = tx.QueryRow(ctx, `SELECT f.file_id,b.name,b.sha256,b.size,b.media_type,b.object_key FROM spatial_version_files f JOIN spatial_blobs b ON b.id=f.blob_id AND b.tenant_id=f.tenant_id WHERE f.tenant_id=$1 AND f.version_id=$2 AND f.file_id=$3`, auth.Tenant, version, id).Scan(&out.ID, &out.Name, &out.SHA256, &out.Size, &out.MediaType, &key)
	}
	if e == pgx.ErrNoRows {
		e = notFound()
	}
	return out, key, e
}
func (s *Service) OpenFile(ctx context.Context, a access.Actor, scope Scope, version, id string, source bool) (*Download, error) {
	action := "versions.files"
	if source {
		action = "versions.source"
	}
	d := &Download{service: s, actor: a, scope: scope, version: version, id: id, action: action}
	var key string
	e := s.transact(ctx, a, scope, action, false, func(tx pgx.Tx, auth k.Authorization) error {
		var e error
		d.File, key, e = s.fileTx(ctx, tx, auth, version, id, source)
		if e != nil || !source {
			return e
		}
		d.observer, e = s.access.BeginTransferTx(ctx, tx, a.ID, auth.TenantMembership+":"+auth.ProjectMembership, ar.Scope{Kind: scope.Entry, Tenant: scope.Tenant, Project: scope.Project}, "spatial", ar.Link{Kind: "domain_file", Module: "spatial", TargetKind: "source", TargetID: version, Revision: version, Relation: "input"})
		return e
	})
	if e != nil {
		return nil, e
	}
	ok := false
	defer func() {
		if !ok {
			_ = d.Close()
		}
	}()
	if s.blobs == nil {
		return nil, unavailable()
	}
	r, e := s.blobs.Get(ctx, key)
	if e != nil {
		if errors.Is(e, fs.ErrNotExist) {
			return nil, s.unavailableVersion(ctx, version)
		}
		return nil, unavailable()
	}
	defer r.Close()
	file, e := os.CreateTemp(s.temp, "spatial-read-*")
	if e != nil {
		return nil, unavailable()
	}
	d.file = file
	h := sha256.New()
	n, e := io.Copy(io.MultiWriter(file, h), io.LimitReader(r, d.Size+1))
	if e != nil {
		d.Close()
		return nil, unavailable()
	}
	if n != d.Size || hex.EncodeToString(h.Sum(nil)) != d.SHA256 {
		d.Close()
		return nil, s.unavailableVersion(ctx, version)
	}
	e = s.transact(ctx, a, scope, action, false, func(tx pgx.Tx, auth k.Authorization) error {
		_, _, e := s.fileTx(ctx, tx, auth, version, id, source)
		if e != nil {
			return e
		}
		return nil
	})
	if e != nil {
		d.Close()
		return nil, e
	}
	ok = true
	return d, nil
}
func (s *Service) unavailableVersion(ctx context.Context, id string) error {
	_ = s.access.Transact(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `UPDATE spatial_asset_versions SET content_available=false WHERE id=$1`, id)
		return e
	})
	return s.integrityError(id)
}

// Every bounded chunk is reauthorized while holding a short authorization read
// transaction. Revocation waits only for that chunk, and stops subsequent bytes.
func (d *Download) Stream(ctx context.Context, start, length int64, consume func([]byte) error) error {
	return d.StreamWrite(ctx, start, length, func(b []byte) (int, error) {
		e := consume(b)
		if e != nil {
			return 0, e
		}
		return len(b), nil
	})
}
func (d *Download) StreamWrite(ctx context.Context, start, length int64, consume func([]byte) (int, error)) (result error) {
	defer func() { d.observer.End(result) }()
	if d.file == nil {
		return unavailable()
	}
	if start < 0 || length < 0 || start+length > d.Size {
		return invalid("/range", "请求范围超出文件。")
	}
	if _, e := d.file.Seek(start, 0); e != nil {
		return e
	}
	buffer := make([]byte, 64*1024)
	for length > 0 {
		n := int64(len(buffer))
		if n > length {
			n = length
		}
		read, e := io.ReadFull(d.file, buffer[:n])
		if e != nil {
			return e
		}
		e = d.service.transact(ctx, d.actor, d.scope, d.action, false, func(tx pgx.Tx, auth k.Authorization) error {
			file, _, e := d.service.fileTx(ctx, tx, auth, d.version, d.id, d.action == "versions.source")
			if e != nil {
				return e
			}
			if file.SHA256 != d.SHA256 {
				return d.service.integrityError(d.version)
			}
			if e = d.observer.HeartbeatTx(ctx, tx); e != nil {
				return e
			}
			n, e := consume(buffer[:read])
			d.observer.Add(n)
			if e == nil && n != read {
				return io.ErrShortWrite
			}
			return e
		})
		if e != nil {
			return e
		}
		length -= int64(read)
	}
	return nil
}
