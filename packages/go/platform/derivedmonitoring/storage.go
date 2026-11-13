package derivedmonitoring

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/resultstore"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
)

func (s *Service) WithStorage(storage *monitoring.Storage) *Service {
	s.results = resultstore.New(storage.CalculationTransport())
	return s
}

// InitializeStorage is part of the explicit monitoring storage upgrade command,
// never an API-startup migration. Existing raw layouts are checked separately.
func InitializeStorage(ctx context.Context, storage *monitoring.Storage) error {
	return resultstore.New(storage.CalculationTransport()).Initialize(ctx)
}
func CheckStorage(ctx context.Context, storage *monitoring.Storage) error {
	return resultstore.New(storage.CalculationTransport()).Check(ctx)
}
