package tenancyhttp

import (
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tenancy"
	identityhttp "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/identity"
	"github.com/gin-gonic/gin"
)

func registerMembership(r *gin.Engine, i *identity.Service, s *tenancy.Service) {
	actor := func(c *gin.Context) access.Actor { return i.AccessActor(identityhttp.Principal(c)) }
	meta := func(c *gin.Context) access.Meta {
		return access.Meta{ActorType: "account", IP: c.ClientIP(), UserAgent: c.Request.UserAgent(), RequestID: c.Writer.Header().Get("X-Request-ID")}
	}
	r.GET("/api/v1/tenants/:tenant_id/members", query("q", "status", "role", "cursor", "limit"), func(c *gin.Context) {
		v, e := s.Members(c.Request.Context(), actor(c), c.Param("tenant_id"), filter(c))
		reply(c, v, e)
	})
	r.GET("/api/v1/tenants/:tenant_id/members/:membership_id", query(), func(c *gin.Context) {
		v, e := s.Member(c.Request.Context(), actor(c), c.Param("tenant_id"), c.Param("membership_id"))
		reply(c, v, e)
	})
	for _, action := range []struct {
		path string
		op   access.Operation
	}{{"suspend", access.OpTenantMemberSuspend}, {"resume", access.OpTenantMemberResume}, {"remove", access.OpTenantMemberRemove}} {
		r.POST("/api/v1/tenants/:tenant_id/members/:membership_id/"+action.path, query(), func(c *gin.Context) {
			in, ok := decode[tenancy.MemberInput](c)
			if !ok {
				return
			}
			v, e := s.ChangeMember(c.Request.Context(), actor(c), c.Param("tenant_id"), c.Param("membership_id"), action.op, in, meta(c))
			reply(c, v, e)
		})
	}
	r.POST("/api/v1/tenants/:tenant_id/member-candidates/resolve", query(), func(c *gin.Context) {
		in, ok := decode[struct {
			Username string `json:"username"`
		}](c)
		if !ok {
			return
		}
		v, e := s.ResolveMember(c.Request.Context(), actor(c), c.Param("tenant_id"), in.Username)
		reply(c, v, e)
	})
	r.GET("/api/v1/tenants/:tenant_id/member-invitations", query("q", "status", "from", "to", "cursor", "limit"), func(c *gin.Context) {
		v, e := s.Invitations(c.Request.Context(), actor(c), c.Param("tenant_id"), filter(c))
		reply(c, v, e)
	})
	r.GET("/api/v1/tenants/:tenant_id/member-invitations/:invitation_id", query(), func(c *gin.Context) {
		v, e := s.Invitation(c.Request.Context(), actor(c), c.Param("tenant_id"), c.Param("invitation_id"))
		reply(c, v, e)
	})
	r.POST("/api/v1/tenants/:tenant_id/member-invitations", query(), func(c *gin.Context) {
		in, ok := decode[tenancy.InvitationCreateInput](c)
		if !ok {
			return
		}
		v, e := s.CreateInvitation(c.Request.Context(), actor(c), c.Param("tenant_id"), in, meta(c))
		reply(c, v, e)
	})
	r.POST("/api/v1/tenants/:tenant_id/member-invitations/:invitation_id/revoke", query(), func(c *gin.Context) {
		in, ok := decode[tenancy.MemberInput](c)
		if !ok {
			return
		}
		v, e := s.RevokeInvitation(c.Request.Context(), actor(c), c.Param("tenant_id"), c.Param("invitation_id"), in, meta(c))
		reply(c, v, e)
	})
	r.POST("/api/v1/tenants/:tenant_id/member-invitations/:invitation_id/regenerate", query(), func(c *gin.Context) {
		in, ok := decode[tenancy.InvitationRegenerateInput](c)
		if !ok {
			return
		}
		v, e := s.RegenerateInvitation(c.Request.Context(), actor(c), c.Param("tenant_id"), c.Param("invitation_id"), in, meta(c))
		reply(c, v, e)
	})
	r.POST("/api/v1/account/tenant-invitations/inspect", query(), func(c *gin.Context) {
		in, ok := decode[struct {
			Token string `json:"token"`
		}](c)
		if !ok {
			return
		}
		v, e := s.InspectInvitation(c.Request.Context(), actor(c), in.Token, meta(c))
		reply(c, v, e)
	})
	r.POST("/api/v1/account/tenant-invitations/accept", query(), func(c *gin.Context) {
		in, ok := decode[tenancy.AcceptanceInput](c)
		if !ok {
			return
		}
		v, e := s.AcceptInvitation(c.Request.Context(), actor(c), in, meta(c))
		reply(c, v, e)
	})
}
