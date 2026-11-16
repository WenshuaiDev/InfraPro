package parsing

import (
	"context"
	"encoding/json"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	"github.com/jackc/pgx/v5"
)

// Recovery APIs are operator-only: they are deliberately absent from HTTP and
// require the owning database/storage configuration. Stop writers before capture.
type RecoveryObject struct {
	ID     string `json:"id"`
	Key    string `json:"key"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size_bytes"`
}
type RecoveryManifest struct {
	ContractVersion int              `json:"contract_version"`
	Methods         []c.Entry        `json:"methods"`
	Objects         []RecoveryObject `json:"objects"`
}

func (s *Service) RecoveryManifest(ctx context.Context) (RecoveryManifest, error) {
	out := RecoveryManifest{ContractVersion: 1, Methods: []c.Entry{}, Objects: []RecoveryObject{}}
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `SELECT entry FROM parsing_method_versions ORDER BY method_id,method_version`)
		if e != nil {
			return e
		}
		for rows.Next() {
			var v c.Entry
			if e = rows.Scan(&v); e != nil {
				rows.Close()
				return e
			}
			out.Methods = append(out.Methods, v)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		rows, e = tx.Query(ctx, `SELECT id::text,object_key,sha256,size_bytes FROM parsing_objects WHERE state='available' ORDER BY id`)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var v RecoveryObject
			if e = rows.Scan(&v.ID, &v.Key, &v.SHA256, &v.Size); e != nil {
				return e
			}
			out.Objects = append(out.Objects, v)
		}
		return rows.Err()
	})
	return out, e
}
func (s *Service) VerifyRecovery(ctx context.Context, directory string) (map[string]int, error) {
	manifest, e := s.RecoveryManifest(ctx)
	if e != nil {
		return nil, e
	}
	if _, e = c.VerifyPackage(directory, manifest.Methods); e != nil {
		return nil, e
	}
	result := map[string]int{"methods": len(manifest.Methods), "objects": len(manifest.Objects), "verified": 0, "missing_or_corrupt": 0}
	for _, v := range manifest.Objects {
		_, e = s.readObject(ctx, object{ID: v.ID, Key: v.Key, Hash: v.SHA256, Size: v.Size, State: "available"}, c.MaxResponse)
		if e != nil {
			result["missing_or_corrupt"]++
		} else {
			result["verified"]++
		}
	}
	return result, nil
}

// Interrupted generations become failed, keeping original attempts and frozen
// input. A human retry creates a new generation under their current authority.
func (s *Service) ReconcileRecovery(ctx context.Context) (int, error) {
	count := 0
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `SELECT r.id::text,r.tenant_id::text FROM parsing_runs r JOIN parsing_run_executions e ON e.run_id=r.id AND e.generation=r.current_generation WHERE e.state='running' ORDER BY r.id FOR UPDATE OF r,e`)
		if e != nil {
			return e
		}
		runs := []Run{}
		for rows.Next() {
			var r Run
			if e = rows.Scan(&r.ID, &r.TenantID); e != nil {
				rows.Close()
				return e
			}
			runs = append(runs, r)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		for _, candidate := range runs {
			r, e := runTx(ctx, tx, runScope(candidate), candidate.ID)
			if e != nil {
				return e
			}
			x, e := executionTx(ctx, tx, r.ID, r.Generation)
			if e != nil {
				return e
			}
			if _, e = tx.Exec(ctx, `UPDATE parsing_run_attempts SET state='failed',error_code='restore_interrupted',finished_at=clock_timestamp() WHERE run_id=$1 AND generation=$2 AND state='running'`, r.ID, r.Generation); e != nil {
				return e
			}
			if e = s.terminalTx(ctx, tx, r, x, "failed", "restore_interrupted", nil, "", nil); e != nil {
				return e
			}
			count++
		}
		return nil
	})
	return count, e
}
func (m RecoveryManifest) Bytes() ([]byte, error) { return json.MarshalIndent(m, "", "  ") }
