package identityhttp

import (
	"io"
	"net/http"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/gin-gonic/gin"
)

func (h *Handler) registerAvatars(r *gin.Engine) {
	auth := r.Group("/api/v1", h.Guard)
	auth.PUT("/account/avatar", func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, identity.AvatarMaxBytes)
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			RespondError(c, identity.FieldError("image", "图片不能超过 5 MB。"))
			return
		}
		account, err := h.service.UpdateAvatar(c.Request.Context(), Principal(c), c.Query("expected_revision"), body, metadata(c))
		reply(c, account, err)
	})
	auth.DELETE("/account/avatar", func(c *gin.Context) {
		account, err := h.service.RemoveAvatar(c.Request.Context(), Principal(c), c.Query("expected_revision"), metadata(c))
		reply(c, account, err)
	})
	for _, route := range []struct{ path, kind string }{
		{"/account/avatar", "self"},
		{"/access/accounts/:id/avatar", "platform"},
		{"/tenants/:tenant_id/members/:membership_id/avatar", "tenant"},
		{"/tenants/:tenant_id/projects/:project_id/members/:membership_id/avatar", "project"},
	} {
		auth.GET(route.path, func(c *gin.Context) {
			data, err := h.service.ReadAvatar(c.Request.Context(), Principal(c), identity.AvatarScope{Kind: route.kind, Tenant: c.Param("tenant_id"), Project: c.Param("project_id"), ID: avatarTarget(c)}, c.Query("version"))
			if err != nil {
				RespondError(c, err)
				return
			}
			c.Header("Cache-Control", "private, no-store")
			c.Header("X-Content-Type-Options", "nosniff")
			c.Data(http.StatusOK, "image/png", data)
		})
	}
}

func avatarTarget(c *gin.Context) string {
	if id := c.Param("membership_id"); id != "" {
		return id
	}
	return c.Param("id")
}
