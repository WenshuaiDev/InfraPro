package monitoring

import (
	"context"
	"time"
)

// TaskScheduler extends the existing worker's scheduling loop, without owning
// a second task queue or keeping durable progress in the process.
type TaskScheduler interface {
	Tick(context.Context, time.Time) error
}

func (s *Service) WithTaskScheduler(scheduler TaskScheduler) *Service {
	s.scheduler = scheduler
	return s
}
