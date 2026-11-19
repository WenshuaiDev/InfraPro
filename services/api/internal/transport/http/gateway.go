package http

import (
	"context"
	"crypto/rand"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/services/api/internal/privateapi"
	"github.com/WenshuaiDev/InfraPro/services/api/internal/routes"
	identityhttp "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/identity"
	"github.com/gin-gonic/gin"
)

const ReadyPath = "/internal/v1/ready"

func Gateway(cfg config.Config, network *privateapi.Network, logger *slog.Logger) http.Handler {
	gin.SetMode(gin.ReleaseMode)
	matcher := gin.New()
	matcher.RedirectTrailingSlash = false
	fallback := gin.New()
	fallback.HandleMethodNotAllowed = true
	fallback.Use(func(c *gin.Context) {
		if c.FullPath() == "" {
			verifyPrincipal(c, network.Verify)
		}
	})
	fallback.NoRoute(func(c *gin.Context) { identityhttp.RespondError(c, identity.ErrNotFound) })
	fallback.NoMethod(func(c *gin.Context) {
		identityhttp.RespondError(c, &identity.Error{Code: "method_not_allowed", Message: "请求方法不支持。", Status: 405})
	})
	ordinary := publicHTTP(cfg, fallback)
	unavailable := publicHTTP(cfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		e := identity.ErrUnavailable
		ginError(w, e)
	}))
	upstreams := map[string]http.Handler{}
	for _, owner := range config.Owners {
		target := network.Arch.Target(owner)
		addr, _ := url.Parse(network.Arch.Endpoints[target])
		proxy := &httputil.ReverseProxy{Transport: network.Transport(target), FlushInterval: -1,
			Rewrite: func(pr *httputil.ProxyRequest) {
				pr.Out.URL.Scheme = addr.Scheme
				pr.Out.URL.Host = addr.Host
				pr.Out.Host = pr.In.Host
				pr.Out.URL.RawQuery = pr.In.URL.RawQuery
				for _, key := range []string{"Forwarded", "X-Forwarded-For", "X-Forwarded-Proto", "X-Forwarded-Host"} {
					if values := pr.In.Header.Values(key); len(values) > 0 {
						pr.Out.Header[key] = values
					}
				}
				for key := range pr.Out.Header {
					if strings.HasPrefix(strings.ToLower(key), "x-infrapro-") {
						pr.Out.Header.Del(key)
					}
				}
				pr.Out.Header.Set(privateapi.PeerAddress, pr.In.RemoteAddr)
				pr.Out.Header.Set(privateapi.RequestID, privateapi.Trace(pr.In.Context()))
			}, ErrorHandler: func(w http.ResponseWriter, r *http.Request, _ error) {
				logger.Warn("upstream unavailable", "owner", owner)
				unavailable.ServeHTTP(w, r)
			},
		}
		slots := make(chan struct{}, network.Arch.Inflight(owner))
		upstreams[owner] = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			case <-r.Context().Done():
				return
			default:
				unavailable.ServeHTTP(w, r)
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), network.Arch.Timeout(owner))
			defer cancel()
			proxy.ServeHTTP(w, r.WithContext(ctx))
		})
	}
	// Gin emits Allow in method-tree insertion order, which is public behavior.
	methodOrder := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD"}
	ops := slices.Clone(routes.Operations)
	slices.SortStableFunc(ops, func(a, b routes.Operation) int {
		return slices.Index(methodOrder, a.Method) - slices.Index(methodOrder, b.Method)
	})
	for _, op := range ops {
		var h http.Handler
		switch op.Path {
		case "/healthz", "/readyz":
			h = publicHTTP(cfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if op.Path == "/readyz" && !gatewayReady(r.Context(), network) {
					e := identity.ErrUnavailable
					ginError(w, e)
					return
				}
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				_, _ = w.Write([]byte(`{"status":"ok"}`))
			}))
		default:
			h = upstreams[op.Owner]
		}
		matcher.Handle(op.Method, op.Path, gin.WrapH(h))
		fallback.Handle(op.Method, op.Path, func(c *gin.Context) { c.AbortWithStatus(404) })
	}
	matcher.NoRoute(gin.WrapH(ordinary))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(privateapi.WithTrace(r.Context(), rand.Text()))
		matcher.ServeHTTP(w, r)
	})
}

func upstreamStatus(ctx context.Context, n *privateapi.Network) map[string]bool {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	type probe struct {
		role  string
		ready bool
	}
	results := make(chan probe, len(config.Owners))
	status := make(map[string]bool, len(config.Owners))
	for _, owner := range config.Owners {
		status[owner] = false
		go func() {
			var ok bool
			err := n.Call(ctx, n.Arch.Target(owner), ReadyPath, struct{}{}, &ok, 1024)
			results <- probe{owner, err == nil && ok}
		}()
	}
	for range config.Owners {
		select {
		case r := <-results:
			status[r.role] = r.ready
		case <-ctx.Done():
			return status
		}
	}
	return status
}
func gatewayReady(ctx context.Context, n *privateapi.Network) bool {
	for _, ready := range upstreamStatus(ctx, n) {
		if !ready {
			return false
		}
	}
	return true
}

// GatewayProbes runs on its own authenticated listener. Operators can drain an
// individual upstream while the public compatibility readiness is degraded.
func GatewayProbes(n *privateapi.Network) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /internal/v1/live", func(w http.ResponseWriter, _ *http.Request) { privateapi.Write(w, true) })
	mux.HandleFunc("GET /internal/v1/upstreams", func(w http.ResponseWriter, r *http.Request) {
		ctx := privateapi.WithTrace(r.Context(), rand.Text())
		privateapi.Write(w, upstreamStatus(ctx, n))
	})
	return privateapi.Allow([]string{config.Gateway, "diagnostics-observer"}, mux)
}

// FromGateway restores the socket address used by the existing trusted-proxy
// policy only after the gateway's client certificate has been authenticated.
func FromGateway(h http.Handler) http.Handler {
	return privateapi.Allow([]string{config.Gateway}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if peer := r.Header.Get(privateapi.PeerAddress); peer != "" {
			if _, _, err := net.SplitHostPort(peer); err == nil {
				r.RemoteAddr = peer
			}
		}
		ctx := privateapi.WithTrace(r.Context(), r.Header.Get(privateapi.RequestID))
		h.ServeHTTP(w, r.WithContext(ctx))
	}))
}
