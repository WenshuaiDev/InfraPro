package tenancyhttp

import (
	"strings"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	identityhttp "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/identity"
	"github.com/gin-gonic/gin"
)

func registerRoles(r *gin.Engine, i *identity.Service) {
	s := i.Access()
	actor := func(c *gin.Context) access.Actor { return i.AccessActor(identityhttp.Principal(c)) }
	meta := func(c *gin.Context) access.Meta {
		return access.Meta{IP: c.ClientIP(), UserAgent: c.Request.UserAgent(), RequestID: c.Writer.Header().Get("X-Request-ID")}
	}
	f := func(c *gin.Context) access.Filter {
		x := filter(c)
		return access.Filter{Query: x.Query, Cursor: x.Cursor, Limit: x.Limit, Status: x.Status, ObjectType: c.Query("source")}
	}
	base := "/api/v1/tenants/:tenant_id"
	r.GET(base+"/permissions", query(), func(c *gin.Context) {
		v, e := s.TenantPermissions(c.Request.Context(), actor(c), c.Param("tenant_id"))
		reply(c, v, e)
	})
	r.GET(base+"/roles", query("q", "cursor", "limit", "status", "source"), func(c *gin.Context) {
		v, e := s.TenantRoles(c.Request.Context(), actor(c), c.Param("tenant_id"), f(c))
		reply(c, v, e)
	})
	r.GET(base+"/roles/:role_id", query(), func(c *gin.Context) {
		v, e := s.TenantRole(c.Request.Context(), actor(c), c.Param("tenant_id"), c.Param("role_id"))
		reply(c, v, e)
	})
	r.GET(base+"/roles/:role_id/members", query("cursor", "limit"), func(c *gin.Context) {
		v, e := s.TenantRoleMembers(c.Request.Context(), actor(c), c.Param("tenant_id"), c.Param("role_id"), f(c))
		reply(c, v, e)
	})
	r.GET("/api/v1/access/tenant-role-presets", query("q", "cursor", "limit", "status"), func(c *gin.Context) { v, e := s.TenantPresets(c.Request.Context(), actor(c), f(c)); reply(c, v, e) })
	r.GET("/api/v1/access/tenant-role-presets/:role_id", query(), func(c *gin.Context) {
		v, e := s.TenantPreset(c.Request.Context(), actor(c), c.Param("role_id"))
		reply(c, v, e)
	})
	r.GET(base+"/members/:membership_id/authorization", query(), func(c *gin.Context) {
		v, e := s.TenantMemberAuthorization(c.Request.Context(), actor(c), c.Param("tenant_id"), c.Param("membership_id"))
		reply(c, v, e)
	})
	for _, path := range []string{"/roles", "/roles/:role_id/copy"} {
		r.POST(base+path, query(), func(c *gin.Context) {
			in, ok := decode[access.TenantRoleCreateInput](c)
			if !ok {
				return
			}
			v, e := s.CreateTenantRole(c.Request.Context(), actor(c), c.Param("tenant_id"), c.Param("role_id"), in, meta(c))
			if e != nil {
				reply(c, v, e)
				return
			}
			c.Header("X-Authorization-Revision", v.Revision)
			status := 201
			if v.Replayed {
				status = 200
			}
			c.JSON(status, v)
		})
	}
	for _, action := range []struct {
		path, method string
		op, preview  access.Operation
	}{
		{"/roles/:role_id", "PATCH", access.OpTenantRoleUpdate, ""},
		{"/roles/:role_id/permissions", "PUT", access.OpTenantRolePermissions, access.OpTenantRolePermissionsPreview},
		{"/roles/:role_id/status", "PUT", access.OpTenantRoleStatus, access.OpTenantRoleStatusPreview},
		{"/roles/:role_id/archive", "POST", access.OpTenantRoleArchive, access.OpTenantRoleArchivePreview},
		{"/members/:membership_id/roles", "PUT", access.OpTenantMemberRoles, access.OpTenantMemberRolesPreview},
	} {
		handle := func(preview bool) gin.HandlerFunc {
			return func(c *gin.Context) {
				in, ok := decodeRoleChange(c, action.op)
				if !ok {
					return
				}
				id := c.Param("role_id")
				if id == "" {
					id = c.Param("membership_id")
				}
				if preview {
					v, e := s.PreviewTenantChange(c.Request.Context(), actor(c), c.Param("tenant_id"), id, action.preview, in, f(c))
					reply(c, v, e)
					return
				}
				v, e := s.ChangeTenantAuthorization(c.Request.Context(), actor(c), c.Param("tenant_id"), id, action.op, in, meta(c))
				if e == nil {
					c.Header("X-Authorization-Revision", v.Revision)
				}
				reply(c, v, e)
			}
		}
		r.Handle(action.method, base+action.path, query(), handle(false))
		if action.preview != "" {
			r.POST(base+action.path+"/preview", query("cursor", "limit"), handle(true))
		}
	}
}

// Each operation has its own strict body: accepted fields cannot smuggle another action.
func decodeRoleChange(c *gin.Context, op access.Operation) (access.TenantChange, bool) {
	var out access.TenantChange
	type proof struct {
		ExpectedVersion string `json:"expected_version"`
		PreviewHash     string `json:"preview_hash"`
		Reason          string `json:"reason"`
	}
	switch op {
	case access.OpTenantRoleUpdate:
		v, ok := decode[struct {
			ExpectedVersion string `json:"expected_version"`
			Name            string `json:"name"`
			Description     string `json:"description"`
			Reason          string `json:"reason"`
		}](c)
		out.ExpectedVersion = v.ExpectedVersion
		out.Name = v.Name
		out.Description = v.Description
		out.Reason = v.Reason
		return out, ok
	case access.OpTenantRolePermissions:
		v, ok := decode[struct {
			proof
			Permissions []access.Code `json:"permission_codes"`
		}](c)
		out.ExpectedVersion = v.ExpectedVersion
		out.PreviewHash = v.PreviewHash
		out.Reason = v.Reason
		out.Permissions = v.Permissions
		return out, ok
	case access.OpTenantRoleStatus:
		v, ok := decode[struct {
			proof
			Status string `json:"status"`
		}](c)
		out.ExpectedVersion = v.ExpectedVersion
		out.PreviewHash = v.PreviewHash
		out.Reason = v.Reason
		out.Status = v.Status
		return out, ok
	case access.OpTenantRoleArchive:
		v, ok := decode[proof](c)
		out.ExpectedVersion = v.ExpectedVersion
		out.PreviewHash = v.PreviewHash
		out.Reason = v.Reason
		return out, ok
	case access.OpTenantMemberRoles:
		v, ok := decode[struct {
			Version  string            `json:"expected_membership_version"`
			Roles    []string          `json:"role_ids"`
			Versions map[string]string `json:"role_versions"`
			Hash     string            `json:"preview_hash"`
			Reason   string            `json:"reason"`
		}](c)
		out.ExpectedMembershipVersion = v.Version
		out.RoleIDs = v.Roles
		out.RoleVersions = v.Versions
		out.PreviewHash = v.Hash
		out.Reason = strings.TrimSpace(v.Reason)
		return out, ok
	}
	return out, false
}
