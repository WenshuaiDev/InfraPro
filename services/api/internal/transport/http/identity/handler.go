package identityhttp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/config"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/csrf"
)

type Handler struct {
	service *identity.Service
	config  config.Config
}

func New(service *identity.Service, cfg config.Config) *Handler {
	return &Handler{service: service, config: cfg}
}

const SessionCookie = "infrapro_session"
const principalKey = "identity.principal"

func SetPrincipal(c *gin.Context, p identity.Principal) { c.Set(principalKey, p) }
func Principal(c *gin.Context) identity.Principal {
	return c.MustGet(principalKey).(identity.Principal)
}
func metadata(c *gin.Context) identity.Meta {
	return identity.Meta{IP: c.ClientIP(), UserAgent: c.Request.UserAgent(), RequestID: c.Writer.Header().Get("X-Request-ID")}
}
func RespondError(c *gin.Context, err error) {
	e := identity.PublicError(err)
	if e.Status == 429 {
		c.Header("Retry-After", "60")
	}
	result := gin.H{"code": e.Code, "message": e.Message, "request_id": c.Writer.Header().Get("X-Request-ID")}
	if e.Fields != nil {
		result["fields"] = e.Fields
	}
	c.AbortWithStatusJSON(e.Status, result)
}
func decode[T any](c *gin.Context) (T, bool) {
	var v T
	d := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 16*1024))
	d.DisallowUnknownFields()
	if d.Decode(&v) != nil {
		RespondError(c, identity.FieldError("request", "请求内容无效，请检查填写内容。"))
		return v, false
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		RespondError(c, identity.FieldError("request", "请求只能包含一个 JSON 对象。"))
		return v, false
	}
	return v, true
}
func reply(c *gin.Context, v any, e error) {
	if e != nil {
		RespondError(c, e)
		return
	}
	if v == nil {
		c.Status(http.StatusNoContent)
	} else {
		c.JSON(http.StatusOK, v)
	}
}
func setSession(c *gin.Context, token string, expiry time.Time) {
	http.SetCookie(c.Writer, &http.Cookie{Name: SessionCookie, Value: token, Path: "/", HttpOnly: true, Secure: false, SameSite: http.SameSiteLaxMode, Expires: expiry, MaxAge: int(time.Until(expiry).Seconds())})
}
func clearSession(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{Name: SessionCookie, Value: "", Path: "/", HttpOnly: true, Secure: false, SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0)})
}
func (h *Handler) Guard(c *gin.Context) {
	if e := h.service.Ready(c.Request.Context()); e != nil {
		RespondError(c, e)
		return
	}
	p, e := h.service.Authenticate(c.Request.Context(), cookie(c))
	if e != nil {
		RespondError(c, e)
		return
	}
	c.Set(principalKey, p)
	c.Next()
}
func cookie(c *gin.Context) string { v, _ := c.Cookie(SessionCookie); return v }
func (h *Handler) PublicReady(c *gin.Context) {
	if e := h.service.Ready(c.Request.Context()); e != nil {
		RespondError(c, e)
		return
	}
	c.Next()
}
func (h *Handler) Register(r *gin.Engine) {
	pub := r.Group("/api/v1", h.PublicReady)
	pub.GET("/public/account-help", func(c *gin.Context) {
		c.JSON(200, gin.H{"support_contact": h.config.SupportContact, "console_url": h.config.PublicConsoleURL, "password_min": identity.PasswordMin, "password_max": identity.PasswordMax, "username_pattern": "^[A-Za-z0-9_-]{3,32}$"})
	})
	pub.GET("/auth/csrf", func(c *gin.Context) { c.JSON(200, gin.H{"token": csrf.Token(c.Request)}) })
	pub.POST("/auth/login", func(c *gin.Context) {
		v, ok := decode[struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}](c)
		if !ok {
			return
		}
		a, token, expiry, e := h.service.Login(c.Request.Context(), v.Username, v.Password, metadata(c))
		if e == nil {
			setSession(c, token, expiry)
		}
		reply(c, a, e)
	})
	pub.POST("/auth/logout", func(c *gin.Context) {
		e := h.service.Logout(c.Request.Context(), cookie(c), metadata(c))
		if e == nil {
			clearSession(c)
		}
		reply(c, nil, e)
	})
	for _, reset := range []bool{false, true} {
		path := "/public/invitations/inspect"
		if reset {
			path = "/public/password-resets/inspect"
		}
		pub.POST(path, func(c *gin.Context) {
			v, ok := decode[struct {
				Token string `json:"token"`
			}](c)
			if !ok {
				return
			}
			result, e := h.service.Inspect(c.Request.Context(), v.Token, reset, metadata(c))
			reply(c, result, e)
		})
	}
	pub.POST("/public/invitations/accept", func(c *gin.Context) {
		if cookie(c) != "" {
			_, e := h.service.Authenticate(c.Request.Context(), cookie(c))
			if e == nil {
				RespondError(c, &identity.Error{Code: "already_signed_in", Message: "请先退出当前账号，再创建新账号。", Status: 409})
				return
			}
			if !errors.Is(e, identity.ErrUnauthorized) {
				RespondError(c, e)
				return
			}
		}
		v, ok := decode[struct {
			Token       string `json:"token"`
			Username    string `json:"username"`
			DisplayName string `json:"display_name"`
			Password    string `json:"password"`
		}](c)
		if !ok {
			return
		}
		reply(c, nil, h.service.AcceptInvitation(c.Request.Context(), v.Token, v.Username, v.DisplayName, v.Password, metadata(c)))
	})
	pub.POST("/public/password-resets/complete", func(c *gin.Context) {
		v, ok := decode[struct {
			Token    string `json:"token"`
			Password string `json:"password"`
		}](c)
		if !ok {
			return
		}
		e := h.service.CompleteReset(c.Request.Context(), v.Token, v.Password, metadata(c))
		if e == nil {
			clearSession(c)
		}
		reply(c, nil, e)
	})
	auth := r.Group("/api/v1")
	auth.GET("/auth/me", func(c *gin.Context) { a, e := h.service.AuthContext(c.Request.Context(), Principal(c)); reply(c, a, e) })
	auth.POST("/auth/reauthenticate", func(c *gin.Context) {
		v, ok := decode[struct {
			Password string `json:"password"`
		}](c)
		if !ok {
			return
		}
		reply(c, nil, h.service.Reauthenticate(c.Request.Context(), Principal(c), v.Password, metadata(c)))
	})
	auth.POST("/auth/logout-all", func(c *gin.Context) {
		e := h.service.Revoke(c.Request.Context(), Principal(c), "", metadata(c))
		if e == nil {
			clearSession(c)
		}
		reply(c, nil, e)
	})
	auth.POST("/auth/activity", func(c *gin.Context) { reply(c, nil, h.service.Activity(c.Request.Context(), Principal(c))) })
	auth.GET("/account/profile", func(c *gin.Context) { v, e := h.service.ReadProfile(c.Request.Context(), Principal(c)); reply(c, v, e) })
	auth.PATCH("/account/profile", func(c *gin.Context) {
		v, ok := decode[struct {
			DisplayName string `json:"display_name"`
		}](c)
		if !ok {
			return
		}
		a, e := h.service.Profile(c.Request.Context(), Principal(c), v.DisplayName, metadata(c))
		reply(c, a, e)
	})
	auth.POST("/account/password", func(c *gin.Context) {
		v, ok := decode[struct {
			Current string `json:"current_password"`
			New     string `json:"new_password"`
		}](c)
		if !ok {
			return
		}
		e := h.service.ChangePassword(c.Request.Context(), Principal(c), v.Current, v.New, metadata(c))
		if e == nil {
			clearSession(c)
		}
		reply(c, nil, e)
	})
	auth.GET("/account/sessions", func(c *gin.Context) { v, e := h.service.Sessions(c.Request.Context(), Principal(c)); reply(c, v, e) })
	auth.DELETE("/account/sessions/:id", func(c *gin.Context) {
		p := Principal(c)
		e := h.service.Revoke(c.Request.Context(), p, c.Param("id"), metadata(c))
		if e == nil && c.Param("id") == p.Session.ID {
			clearSession(c)
		}
		reply(c, nil, e)
	})
	auth.GET("/account/security-events", func(c *gin.Context) {
		v, e := h.service.SecurityEvents(c.Request.Context(), Principal(c), c.Query("cursor"))
		reply(c, v, e)
	})
	auth.GET("/invitations", func(c *gin.Context) {
		v, e := h.service.Invitations(c.Request.Context(), Principal(c), identity.InvitationFilter{Cursor: c.Query("cursor"), Query: c.Query("q"), Status: c.Query("status")})
		reply(c, v, e)
	})
	auth.GET("/invitations/:id", func(c *gin.Context) {
		v, e := h.service.Invitation(c.Request.Context(), Principal(c), c.Param("id"))
		reply(c, v, e)
	})
	auth.POST("/invitations", func(c *gin.Context) {
		v, ok := decode[struct {
			Note string `json:"note"`
		}](c)
		if !ok {
			return
		}
		link, e := h.service.CreateInvitation(c.Request.Context(), Principal(c), "", v.Note, metadata(c))
		reply(c, link, e)
	})
	auth.POST("/invitations/:id/revoke", func(c *gin.Context) {
		reply(c, nil, h.service.RevokeInvitation(c.Request.Context(), Principal(c), c.Param("id"), metadata(c)))
	})
	auth.POST("/invitations/:id/regenerate", func(c *gin.Context) {
		v, e := h.service.CreateInvitation(c.Request.Context(), Principal(c), c.Param("id"), "", metadata(c))
		reply(c, v, e)
	})
}
