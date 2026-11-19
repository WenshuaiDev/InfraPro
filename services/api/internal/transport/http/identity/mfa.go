package identityhttp

import (
	"errors"
	"github.com/WenshuaiDev/InfraPro/services/api/internal/routes"
	"net/http"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/gin-gonic/gin"
)

const ChallengeCookie = "infrapro_auth_challenge"

func challengeCookie(c *gin.Context) string { v, _ := c.Cookie(ChallengeCookie); return v }
func (h *Handler) setChallenge(c *gin.Context, token string, expiry time.Time) {
	http.SetCookie(c.Writer, &http.Cookie{Name: ChallengeCookie, Value: token, Path: "/api/v1/auth", HttpOnly: true, Secure: h.config.CookieSecure, SameSite: http.SameSiteLaxMode, Expires: expiry, MaxAge: max(1, int(time.Until(expiry).Seconds()))})
}
func (h *Handler) clearChallenge(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{Name: ChallengeCookie, Path: "/api/v1/auth", HttpOnly: true, Secure: h.config.CookieSecure, SameSite: http.SameSiteLaxMode, Expires: time.Unix(1, 0), MaxAge: -1})
}
func (h *Handler) authReply(c *gin.Context, d identity.AuthDelivery, e error) {
	if e != nil {
		RespondError(c, e)
		return
	}
	if d.SessionToken != "" {
		h.setSession(c, d.SessionToken, d.ExpiresAt)
		h.clearChallenge(c)
		c.JSON(200, d.Result)
		return
	}
	h.setChallenge(c, d.ChallengeToken, d.ExpiresAt)
	c.JSON(202, d.Result)
}
func (h *Handler) beginLogin(c *gin.Context) {
	if cookie(c) != "" {
		_, e := h.service.Authenticate(c.Request.Context(), cookie(c))
		if e == nil {
			RespondError(c, &identity.Error{Code: "already_signed_in", Message: "请先退出当前账号，再登录其他账号。", Status: 409})
			return
		}
		if !errors.Is(e, identity.ErrUnauthorized) && !errors.Is(e, identity.ErrMFALogin) {
			RespondError(c, e)
			return
		}
		h.clearSession(c)
	}
	v, ok := decode[struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}](c)
	if !ok {
		return
	}
	d, e := h.service.BeginLogin(c.Request.Context(), v.Username, v.Password, challengeCookie(c), metadata(c))
	h.authReply(c, d, e)
}
func (h *Handler) registerMFA(r routes.Registrar) {
	pre := r.Group("/api/v1/auth/mfa", h.PublicReady)
	pre.GET("/challenge", func(c *gin.Context) {
		v, e := h.service.Challenge(c.Request.Context(), challengeCookie(c))
		reply(c, v, e)
	})
	pre.POST("/verify", func(c *gin.Context) {
		v, ok := decode[struct {
			ID     string `json:"challenge_id"`
			Method string `json:"method"`
			Code   string `json:"code"`
		}](c)
		if !ok {
			return
		}
		d, e := h.service.CompleteLogin(c.Request.Context(), challengeCookie(c), v.ID, v.Method, v.Code, metadata(c))
		h.authReply(c, d, e)
	})
	pre.POST("/enrollment/start", func(c *gin.Context) {
		v, ok := decode[struct {
			ID string `json:"challenge_id"`
		}](c)
		if !ok {
			return
		}
		out, e := h.service.StartLoginEnrollment(c.Request.Context(), challengeCookie(c), v.ID, metadata(c))
		reply(c, out, e)
	})
	pre.POST("/enrollment/confirm", func(c *gin.Context) {
		v, ok := decode[struct {
			ID   string `json:"challenge_id"`
			Code string `json:"new_totp_code"`
		}](c)
		if !ok {
			return
		}
		d, e := h.service.ConfirmEnrollment(c.Request.Context(), nil, challengeCookie(c), v.ID, v.Code, metadata(c))
		h.authReply(c, d, e)
	})
	pre.POST("/recovery/claim", func(c *gin.Context) {
		v, ok := decode[struct {
			ID    string `json:"challenge_id"`
			Token string `json:"recovery_token"`
		}](c)
		if !ok {
			return
		}
		d, e := h.service.ClaimMFARecovery(c.Request.Context(), challengeCookie(c), v.ID, v.Token, metadata(c))
		h.authReply(c, d, e)
	})
	pre.POST("/cancel", func(c *gin.Context) {
		v, ok := decode[struct {
			ID string `json:"challenge_id"`
		}](c)
		if !ok {
			return
		}
		e := h.service.CancelChallenge(c.Request.Context(), challengeCookie(c), v.ID)
		if e == nil {
			h.clearChallenge(c)
		}
		reply(c, nil, e)
	})
	auth := r.Group("/api/v1")
	auth.GET("/account/mfa", func(c *gin.Context) { v, e := h.service.MFA(c.Request.Context(), Principal(c)); reply(c, v, e) })
	auth.POST("/account/mfa/enrollment/start", func(c *gin.Context) {
		v, ok := decode[struct {
			Mode    string `json:"mode"`
			Version string `json:"expected_factor_version"`
		}](c)
		if !ok {
			return
		}
		out, e := h.service.StartSelfEnrollment(c.Request.Context(), Principal(c), v.Mode, v.Version, metadata(c))
		reply(c, out, e)
	})
	auth.POST("/account/mfa/enrollment/confirm", func(c *gin.Context) {
		v, ok := decode[struct {
			ID   string `json:"enrollment_id"`
			Code string `json:"new_totp_code"`
		}](c)
		if !ok {
			return
		}
		p := Principal(c)
		d, e := h.service.ConfirmEnrollment(c.Request.Context(), &p, "", v.ID, v.Code, metadata(c))
		if e != nil {
			RespondError(c, e)
			return
		}
		h.setSession(c, d.SessionToken, d.ExpiresAt)
		status, e := h.service.Authenticate(c.Request.Context(), d.SessionToken)
		if e != nil {
			RespondError(c, e)
			return
		}
		out, e := h.service.MFA(c.Request.Context(), status)
		reply(c, identity.MFAResult{MFAStatus: out, RecoveryCodes: d.Result.RecoveryCodes, OperationID: d.Result.OperationID}, e)
	})
	auth.POST("/account/mfa/enrollment/cancel", func(c *gin.Context) {
		v, ok := decode[struct {
			ID string `json:"enrollment_id"`
		}](c)
		if !ok {
			return
		}
		reply(c, nil, h.service.CancelSelfEnrollment(c.Request.Context(), Principal(c), v.ID))
	})
	auth.POST("/account/mfa/disable", func(c *gin.Context) {
		v, ok := decode[struct {
			Version string `json:"expected_factor_version"`
		}](c)
		if !ok {
			return
		}
		out, d, e := h.service.DisableMFA(c.Request.Context(), Principal(c), v.Version, c.GetHeader("X-Command-ID"), metadata(c))
		if e == nil && d.SessionToken != "" {
			h.setSession(c, d.SessionToken, d.ExpiresAt)
		}
		reply(c, out, e)
	})
	auth.POST("/account/mfa/recovery-codes/regenerate", func(c *gin.Context) {
		v, ok := decode[struct {
			Version    string `json:"expected_factor_version"`
			Generation string `json:"expected_recovery_generation"`
		}](c)
		if !ok {
			return
		}
		out, e := h.service.RegenerateRecoveryCodes(c.Request.Context(), Principal(c), v.Version, v.Generation, c.GetHeader("X-Command-ID"), metadata(c))
		reply(c, out, e)
	})
	auth.GET("/platform/security-policy", func(c *gin.Context) { v, e := h.service.Policy(c.Request.Context(), Principal(c)); reply(c, v, e) })
	auth.GET("/platform/security-policy/mfa-impact", func(c *gin.Context) {
		v, e := h.service.PolicyImpact(c.Request.Context(), Principal(c))
		reply(c, v, e)
	})
	auth.PUT("/platform/security-policy", func(c *gin.Context) {
		v, ok := decode[struct {
			Required *bool  `json:"require_mfa"`
			Version  string `json:"expected_version"`
			Reason   string `json:"reason"`
		}](c)
		if !ok {
			return
		}
		if v.Required == nil {
			RespondError(c, identity.FieldError("require_mfa", "请选择平台策略。"))
			return
		}
		m := metadata(c)
		m.Reason = v.Reason
		out, e := h.service.UpdatePolicy(c.Request.Context(), Principal(c), *v.Required, v.Version, c.GetHeader("X-Command-ID"), m)
		if e == nil && out.SessionAction == "sign_in_again" {
			h.clearSession(c)
		}
		reply(c, out, e)
	})
	auth.GET("/access/accounts/:id/mfa", func(c *gin.Context) {
		v, e := h.service.AccountMFA(c.Request.Context(), Principal(c), c.Param("id"))
		reply(c, v, e)
	})
	auth.POST("/access/accounts/:id/mfa-reset", func(c *gin.Context) {
		v, ok := decode[struct {
			Version string `json:"expected_factor_version"`
			Reason  string `json:"reason"`
		}](c)
		if !ok {
			return
		}
		m := metadata(c)
		m.Reason = v.Reason
		out, e := h.service.ResetMFA(c.Request.Context(), Principal(c), c.Param("id"), v.Version, c.GetHeader("X-Command-ID"), m)
		reply(c, out, e)
	})
}
