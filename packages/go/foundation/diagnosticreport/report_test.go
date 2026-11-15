package diagnosticreport

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestWriterFailureAndHangNeverBlockBusiness(t *testing.T) {
	for _, hung := range []bool{false, true} {
		t.Run(map[bool]string{false: "failure", true: "hung"}[hung], func(t *testing.T) {
			gate := make(chan struct{})
			entered := make(chan struct{}, 1)
			r := newReporter(Report{Phase: "starting"}, func([]byte) error {
				select {
				case entered <- struct{}{}:
				default:
				}
				if hung {
					<-gate
				}
				return errors.New("SECRET must never become a reason")
			})
			<-entered
			start := time.Now()
			for i := 0; i < 1000; i++ {
				r.Begin("jobs", time.Time{})
				r.End("jobs", "loop_failed", time.Now().Add(time.Hour))
				r.Dependency("postgres", false, "SECRET")
			}
			if time.Since(start) > time.Second {
				t.Fatal("sink waited for writer")
			}
			start = time.Now()
			r.Close()
			if time.Since(start) > 300*time.Millisecond {
				t.Fatal("exit waited for writer")
			}
			close(gate)
		})
	}
}
func TestWriterDoesNotAdvanceActualLoopTimes(t *testing.T) {
	r := newReporter(Report{Lanes: []Lane{}, Dependencies: []DependencyState{}}, func([]byte) error { return nil })
	defer r.Close()
	next := time.Now().Add(24 * time.Hour)
	r.Idle("maintenance", next)
	var first, second Report
	_ = json.Unmarshal(r.snapshot(), &first)
	time.Sleep(time.Millisecond)
	_ = json.Unmarshal(r.snapshot(), &second)
	if !first.Lanes[0].LastAttemptAt.Equal(*second.Lanes[0].LastAttemptAt) || !first.Lanes[0].NextAttemptAt.Equal(*second.Lanes[0].NextAttemptAt) {
		t.Fatal("writer advanced the loop")
	}
	r.Begin("maintenance", time.Time{})
	r.End("maintenance", "secret raw error", next)
	raw := r.snapshot()
	if strings.Contains(string(raw), "secret") {
		t.Fatal("unsafe reason persisted")
	}
	for i := 0; i < 100; i++ {
		r.Idle(strings.Repeat("a", i+1), next)
	}
	_ = json.Unmarshal(r.snapshot(), &second)
	if len(second.Lanes) > 16 {
		t.Fatal("unbounded lanes")
	}
}
