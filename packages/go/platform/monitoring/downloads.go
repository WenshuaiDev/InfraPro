package monitoring

import (
	"context"
	"encoding/json"
	"io"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

// Download streams through the authenticated service. It never exposes a long
// lived signed object URL that could outlive a permission revocation.
type Download struct {
	Name        string
	Size        int64
	SHA256      string
	ContentType string
	Write       func(context.Context, io.Writer) error
}

func (s *Service) Source(ctx context.Context, a access.Actor, scope Scope, id string) (Download, error) {
	out := Download{ContentType: "application/octet-stream"}
	if e := s.requireStorage(); e != nil {
		return out, e
	}
	if !access.ValidID(id) {
		return out, notFound()
	}
	var key, revision string
	e := s.transact(ctx, a, scope, "sources.download", false, false, func(tx pgx.Tx, auth authorization) error {
		e := tx.QueryRow(ctx, `SELECT object_key,original_name,size_bytes,sha256 FROM monitoring_source_files WHERE tenant_id=$1 AND id=$2`, scope.Tenant, id).Scan(&key, &out.Name, &out.Size, &out.SHA256)
		if e == pgx.ErrNoRows {
			return notFound()
		}
		if e != nil {
			return e
		}
		revision = authorizationVersion(auth)
		if scope.Entry == "project" {
			var raw []byte
			e = tx.QueryRow(ctx, `SELECT semantic_snapshot FROM monitoring_import_batches WHERE tenant_id=$1 AND source_file_id=$2 AND readopt_batch_id IS NULL AND counts->>'errors'='0' AND semantic_snapshot->>'whole_source'='true' AND validation_hash IS NOT NULL ORDER BY updated_at DESC,id LIMIT 1`, scope.Tenant, id).Scan(&raw)
			if e == pgx.ErrNoRows {
				return notFound()
			}
			if e != nil {
				return e
			}
			var snapshot validationSnapshot
			if e = json.Unmarshal(raw, &snapshot); e != nil {
				return e
			}
			ids := []string{}
			for channel := range snapshot.Coverage {
				ids = append(ids, channel)
			}
			if len(ids) == 0 {
				return notFound()
			}
			windows, e := authorizedWindowsTx(ctx, tx, auth, ids, "source")
			if e != nil {
				return e
			}
			if e = coversBatch(snapshot.Coverage, windows); e != nil {
				return e
			}
		}
		return nil
	})
	if e != nil {
		return out, e
	}
	out.Write = func(ctx context.Context, writer io.Writer) error {
		object, e := s.storage.get(ctx, key)
		if e != nil {
			return storageError(e)
		}
		defer object.Close()
		buffer := make([]byte, 128*1024)
		var written int64
		for {
			// Recheck before every block; a change ends the transfer. Content-Length
			// lets the client distinguish a terminated stream from a complete file.
			if e = s.transact(ctx, a, scope, "sources.download", false, false, func(_ pgx.Tx, auth authorization) error {
				if authorizationVersion(auth) != revision {
					return problem("authorization_changed", "下载期间授权已变化。", 409)
				}
				return nil
			}); e != nil {
				return e
			}
			n, readError := object.Read(buffer)
			if n > 0 {
				count, writeError := writer.Write(buffer[:n])
				written += int64(count)
				if writeError != nil {
					return writeError
				}
				if count != n {
					return io.ErrShortWrite
				}
			}
			if readError == io.EOF {
				if written != out.Size {
					return io.ErrUnexpectedEOF
				}
				return nil
			}
			if readError != nil {
				return storageError(readError)
			}
		}
	}
	return out, nil
}
