package monitoring

import (
	"fmt"
	"time"
)

type WorkerOptions struct {
	Foreground  int
	Historical  int
	Lease       time.Duration
	Heartbeat   time.Duration
	MaxAttempts int
}

func DefaultWorkerOptions() WorkerOptions {
	return WorkerOptions{Foreground: 1, Historical: 1, Lease: 30 * time.Second, Heartbeat: 5 * time.Second, MaxAttempts: 5}
}
func (s *Service) WithWorkerOptions(o WorkerOptions) (*Service, error) {
	if o.Foreground < 1 || o.Historical < 1 || o.Foreground+o.Historical > 32 || o.Lease < 10*time.Second || o.Lease > 5*time.Minute || o.Heartbeat < time.Second || o.Heartbeat*3 > o.Lease || o.MaxAttempts < 1 || o.MaxAttempts > 10 {
		return s, fmt.Errorf("invalid shared monitoring worker options")
	}
	s.workers = o
	return s, nil
}

const taskQueueSQL = `(CASE WHEN kind IN ('validate','publish') OR kind='derive' AND input->>'mode' IN ('trial','automatic','correction') OR kind='analysis' AND input->>'trial'='true' THEN 'foreground' ELSE 'historical' END)`
