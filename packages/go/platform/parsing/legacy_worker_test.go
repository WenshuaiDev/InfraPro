package parsing

import (
	"context"

	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
)

// RunV44Fixture executes the historical fixed-method machine contract inside
// the normal run, attempt, object, and selected-result transactions. The v44
// schema predates cross-replica executor slots. This adapter exists only in
// the test binary that creates evidence on that schema before migrating it.
func (s *Service) RunV44Fixture(ctx context.Context) (bool, error) {
	return s.runOne(ctx, func(ctx context.Context, _, _ string, req c.Request, entry c.Entry) ([]byte, c.Response, error) {
		return s.ExecuteFixed(ctx, req, entry)
	})
}
