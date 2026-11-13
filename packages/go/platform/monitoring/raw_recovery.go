package monitoring

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// VerifyRawCheckpointTx is a maintenance integrity check. It returns no values
// or user authority; revoked historical adoptions remain legitimate evidence.
func VerifyRawCheckpointTx(ctx context.Context, tx pgx.Tx, tenant, id string) error {
	v, e := loadRawEvidenceTx(ctx, tx, tenant, id)
	if e != nil {
		return e
	}
	for _, chunk := range v.Manifest {
		var valid bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM monitoring_chunks c JOIN monitoring_write_attempts a ON a.tenant_id=c.tenant_id AND a.chunk_id=c.id WHERE c.tenant_id=$1 AND c.id=$2 AND a.id=$3 AND c.payload_hash=$4 AND c.row_count=$5 AND a.state='sealed')`, tenant, chunk.Chunk, chunk.Attempt, chunk.Hash, int64(chunk.Count)).Scan(&valid); e != nil {
			return e
		}
		if !valid {
			return fmt.Errorf("raw_checkpoint_attempt_mismatch")
		}
	}
	return nil
}
