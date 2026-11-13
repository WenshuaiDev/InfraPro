package monitoring

import (
	"context"
	"encoding/json"
)

// CalculationMaintenance joins the existing system verification job. It can
// quarantine corrupt immutable results; it conveys no business/user authority.
type CalculationMaintenance interface {
	MaintainCalculations(context.Context, TaskLease) (json.RawMessage, error)
}

func (s *Service) WithCalculationMaintenance(maintenance CalculationMaintenance) *Service {
	s.calculationMaintenance = maintenance
	return s
}
