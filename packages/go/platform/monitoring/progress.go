package monitoring

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// Progress records completed work only. Its lease/fence check is the same as a
// result commit, so a superseded worker cannot overwrite its successor's state.
func (s *Service) recordProgress(ctx context.Context, j Job, progress map[string]string) error {
	recordTaskProgress(ctx, progress)
	return s.jobTransaction(ctx, j, func(tx pgx.Tx) error {
		if err := assertJobTx(ctx, tx, j); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE monitoring_jobs SET progress=$2,updated_at=clock_timestamp() WHERE id=$1`, j.ID, encodeJSON(progress))
		return err
	})
}

type progressWriter struct {
	bytes   int64
	changed func() error
}

func (w *progressWriter) Write(p []byte) (int, error) {
	w.bytes += int64(len(p))
	if w.changed != nil {
		if e := w.changed(); e != nil {
			return len(p), e
		}
	}
	return len(p), nil
}
