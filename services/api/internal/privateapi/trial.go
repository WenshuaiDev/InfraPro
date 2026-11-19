package privateapi

import (
	"context"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/dispatch"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
)

const TrialPath = "/internal/v1/monitoring/trials/attempt"

type trialRequest struct {
	Tenant   string    `json:"tenant_id"`
	Job      string    `json:"job_id"`
	Request  string    `json:"request_id"`
	Deadline time.Time `json:"deadline"`
}
type trialReply struct {
	State string `json:"state"`
}

// AttemptTrial includes queue and transport time in the foreground budget.
func (n *Network) AttemptTrial(ctx context.Context, tenant, id string, budget time.Duration) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	end, _ := ctx.Deadline()
	var out trialReply
	err := n.Call(ctx, string(monitoring.IncrementalWorker), TrialPath, trialRequest{tenant, id, Trace(ctx), end}, &out, 4096)
	if err != nil {
		return false, err
	}
	switch out.State {
	case "executed":
		return true, nil
	case "already_claimed", "background":
		return false, nil
	}
	return false, errors.New("unexpected trial response")
}

func TrialHandler(s *derivedmonitoring.Service, maxBudget time.Duration) http.Handler {
	return Allow([]string{config.Control}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in trialRequest
		if !Read(w, r, 2048, &in) {
			return
		}
		if !access.ValidID(in.Tenant) || !access.ValidID(in.Job) || in.Request == "" || len(in.Request) > 128 || r.Header.Get(RequestID) != in.Request || in.Deadline.IsZero() {
			http.Error(w, "invalid private trial reference", 400)
			return
		}
		budget := min(maxBudget, time.Until(in.Deadline))
		state := "background"
		if budget > 0 {
			claimed, err := s.AttemptTrial(WithTrace(r.Context(), in.Request), in.Tenant, in.Job, budget)
			if err == nil {
				if claimed {
					state = "executed"
				} else {
					state = "already_claimed"
				}
			}
		}
		Write(w, trialReply{state})
	}))
}

// CompatibilityTrials keeps a compatible original-topology API fenced while
// route groups and worker ownership are being switched independently.
func CompatibilityTrials(pool *pgxpool.Pool, s *derivedmonitoring.Service) {
	s.WithTrialAttempt(func(ctx context.Context, tenant, id string, budget time.Duration) (bool, error) {
		ctx, cancel := context.WithTimeout(ctx, budget)
		defer cancel()
		var claimed bool
		_, e := dispatch.Try(ctx, pool, string(monitoring.IncrementalWorker), "compat", func(c context.Context) error {
			var err error
			claimed, err = s.AttemptTrial(c, tenant, id, budget)
			return err
		})
		return claimed, e
	})
}
