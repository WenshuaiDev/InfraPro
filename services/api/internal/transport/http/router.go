package http

import (
	"crypto/rand"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/projects"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/runtime"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tenancy"
	accesshttp "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/access"
	devicecataloghttp "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/devicecatalog"
	deviceshttp "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/devices"
	engineeringhttp "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/engineering"
	projectshttp "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/projects"
	resourceshttp "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/resources"
	sourceshttp "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/sources"
	tenancyhttp "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/tenancy"
	"log/slog"
	"net/http"
	"net/url"
	"slices"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	identityhttp "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/identity"
	"github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/middleware"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/csrf"
)

func NewRouter(logger *slog.Logger, service *identity.Service, cfg config.Config) http.Handler {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	handler := identityhttp.New(service, cfg)
	if e := r.SetTrustedProxies(cfg.TrustedProxies); e != nil {
		panic("invalid trusted proxy configuration")
	}
	r.HandleMethodNotAllowed = true
	r.Use(middleware.RequestLog(logger), gin.CustomRecovery(func(c *gin.Context, _ any) {
		logger.Error("request panic")
		identityhttp.RespondError(c, &identity.Error{Code: "internal_error", Message: "请求未能完成，请稍后重试。", Status: 500})
	}))
	r.Use(func(c *gin.Context) {
		policy, ok := access.RoutePolicy(c.Request.Method, c.FullPath())
		if !ok {
			if c.FullPath() != "" {
				identityhttp.RespondError(c, access.ErrDenied)
				return
			}
			if service == nil {
				identityhttp.RespondError(c, identity.ErrUnavailable)
				return
			}
			handler.Guard(c)
			return
		}
		if policy.Class == "public" || policy.Class == "credential" {
			c.Next()
			return
		}
		if service == nil {
			identityhttp.RespondError(c, identity.ErrUnavailable)
			return
		}
		// Guard does not run downstream until the explicit operation check has passed.
		if e := service.Ready(c.Request.Context()); e != nil {
			identityhttp.RespondError(c, e)
			return
		}
		token, _ := c.Cookie(identityhttp.SessionCookie)
		p, e := service.Authenticate(c.Request.Context(), token)
		if e != nil {
			identityhttp.RespondError(c, e)
			return
		}
		identityhttp.SetPrincipal(c, p)
		if policy.Class == "permission" || policy.Class == "permission_any" || policy.Class == "tenant" {
			rev, e := service.Access().AuthorizeScoped(c.Request.Context(), p.Account.ID, policy.Operation, c.Param("tenant_id"))
			if e != nil {
				identityhttp.RespondError(c, e)
				return
			}
			c.Header("X-Authorization-Revision", rev)
		}
		c.Next()
	})
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/readyz", func(c *gin.Context) {
		if service == nil {
			identityhttp.RespondError(c, identity.ErrUnavailable)
			return
		}
		if e := service.Ready(c.Request.Context()); e != nil {
			identityhttp.RespondError(c, e)
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	if service != nil {
		handler.Register(r)
		accesshttp.Register(r, service)
		projectshttp.Register(r, service, projects.New(service.Access()))
		business := runtime.Compose(service.Access())
		resourceshttp.Register(r, service, business.Resources)
		engineeringhttp.Register(r, service, business.Engineering)
		devicecataloghttp.Register(r, service, business.DeviceCatalog)
		deviceshttp.Register(r, service, business.Devices)
		sourceshttp.Register(r, service, business.Sources)
		tenancyhttp.Register(r, service, tenancy.New(service.Access(), tenancy.Options{ConsoleURL: cfg.PublicConsoleURL, Limit: service.Limit, CredentialLimit: service.Rate}))
		for _, route := range r.Routes() {
			if _, ok := access.RoutePolicy(route.Method, route.Path); !ok {
				panic("route lacks authorization policy: " + route.Method + " " + route.Path)
			}
		}
	}
	r.NoRoute(func(c *gin.Context) { identityhttp.RespondError(c, identity.ErrNotFound) })
	r.NoMethod(func(c *gin.Context) {
		identityhttp.RespondError(c, &identity.Error{Code: "method_not_allowed", Message: "请求方法不支持。", Status: 405})
	})
	origins := []string{}
	for _, o := range cfg.TrustedOrigins {
		u, _ := url.Parse(o)
		origins = append(origins, u.Host)
	}
	wrapped := csrf.Protect(cfg.CSRFKey, csrf.Secure(false), csrf.HttpOnly(true), csrf.Path("/"), csrf.CookieName("infrapro_csrf"), csrf.SameSite(csrf.SameSiteLaxMode), csrf.TrustedOrigins(origins), csrf.ErrorHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, 403, "csrf_invalid", "页面验证已过期，请刷新验证后重新提交。")
	})))(r)
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		id := rand.Text()
		w.Header().Set("X-Request-ID", id)
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if req.Method != "GET" && req.Method != "HEAD" && req.Method != "OPTIONS" {
			if !slices.Contains(cfg.TrustedOrigins, req.Header.Get("Origin")) {
				writeError(w, 403, "origin_rejected", "请求来源不受信任，请从配置的应用地址打开页面。")
				return
			}
		}
		// Configuration, never a client header, selects the plaintext HTTP CSRF mode.
		wrapped.ServeHTTP(w, csrf.PlaintextHTTPRequest(req))
	})
}
func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code, "message": message, "request_id": w.Header().Get("X-Request-ID")})
}
