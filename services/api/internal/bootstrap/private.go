package bootstrap

import (
	"net/http"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/WenshuaiDev/InfraPro/services/api/internal/privateapi"
)

func configurePrivateBusiness(mux *http.ServeMux, b runtime.Business, n *privateapi.Network, cfg config.Config) {
	if cfg.Architecture.Role == config.Control {
		b.DerivedMonitoring.WithTrialAttempt(n.AttemptTrial)
	}
	if cfg.Architecture.Role == config.Query || cfg.Architecture.Role == "compat-api" {
		mux.Handle(privateapi.ReadsPath, privateapi.ReadsHandler(b.Displays))
	}
}
