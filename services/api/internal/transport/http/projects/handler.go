package projectshttp

import (
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/projects"
	identityhttp "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/identity"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
)

func decode[T any](c *gin.Context) (T, bool) {
	var v *T
	d := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 64*1024))
	d.DisallowUnknownFields()
	var rest any
	if d.Decode(&v) != nil || v == nil || d.Decode(&rest) != io.EOF {
		identityhttp.RespondError(c, access.Validation("request", "请求包含不支持的字段或无效内容。"))
		var zero T
		return zero, false
	}
	return *v, true
}
func reply(c *gin.Context, v any, e error) {
	if e != nil {
		identityhttp.RespondError(c, e)
		return
	}
	if r, ok := v.(projects.Result); ok {
		c.Header("X-Authorization-Revision", r.Revision)
	}
	if r, ok := v.(projects.Context); ok {
		c.Header("X-Authorization-Revision", r.Authorization.Revision)
	}
	c.JSON(200, v)
}
func query(allowed ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		for k := range c.Request.URL.Query() {
			if !slices.Contains(allowed, k) {
				identityhttp.RespondError(c, access.Validation(k, "不支持的查询字段。"))
				return
			}
		}
		c.Next()
	}
}
func filter(c *gin.Context) projects.Filter {
	n := 0
	if c.Query("limit") != "" {
		var e error
		n, e = strconv.Atoi(c.Query("limit"))
		if e != nil {
			n = -1
		}
	}
	return projects.Filter{Query: c.Query("q"), Cursor: c.Query("cursor"), Limit: n, Status: c.Query("status"), Owner: c.Query("owner_tenant_membership_id"), Unavailable: c.Query("administrator_unavailable"), Event: c.Query("event_type"), From: c.Query("from"), To: c.Query("to"), ObjectID: c.Query("object_id"), ActorID: c.Query("actor_id")}
}
func Register(r *gin.Engine, i *identity.Service, s *projects.Service) {
	actor := func(c *gin.Context) access.Actor { return i.AccessActor(identityhttp.Principal(c)) }
	meta := func(c *gin.Context) access.Meta {
		return access.Meta{ActorType: "account", IP: c.ClientIP(), UserAgent: c.Request.UserAgent(), RequestID: c.Writer.Header().Get("X-Request-ID")}
	}
	const t = "/api/v1/tenants/:tenant_id"
	const p = t + "/projects/:project_id"
	const g = t + "/project-governance"
	r.GET("/api/v1/me/projects", query("q", "status", "cursor", "limit"), func(c *gin.Context) {
		v, e := s.List(c.Request.Context(), actor(c), "", filter(c), true)
		reply(c, v, e)
	})
	r.GET(g, query("q", "status", "cursor", "limit", "owner_tenant_membership_id", "administrator_unavailable"), func(c *gin.Context) {
		v, e := s.List(c.Request.Context(), actor(c), c.Param("tenant_id"), filter(c), false)
		reply(c, v, e)
	})
	r.GET(g+"/:project_id", query(), func(c *gin.Context) {
		v, e := s.Governance(c.Request.Context(), actor(c), c.Param("tenant_id"), c.Param("project_id"))
		reply(c, v, e)
	})
	r.POST(t+"/projects", query(), func(c *gin.Context) {
		in, ok := decode[projects.CreateInput](c)
		if !ok {
			return
		}
		v, e := s.Create(c.Request.Context(), actor(c), c.Param("tenant_id"), in, meta(c))
		reply(c, v, e)
	})
	for _, path := range []string{p + "/context", p + "/availability"} {
		r.GET(path, query(), func(c *gin.Context) {
			read := s.Context
			if path == p+"/availability" {
				read = s.Availability
			}
			v, e := read(c.Request.Context(), actor(c), c.Param("tenant_id"), c.Param("project_id"))
			reply(c, v, e)
		})
	}
	r.GET(p+"/profile", query(), func(c *gin.Context) {
		v, e := s.Profile(c.Request.Context(), actor(c), c.Param("tenant_id"), c.Param("project_id"))
		reply(c, v, e)
	})
	r.PATCH(p+"/profile", query(), func(c *gin.Context) {
		in, ok := decode[projects.UpdateInput](c)
		if !ok {
			return
		}
		v, e := s.Update(c.Request.Context(), actor(c), c.Param("tenant_id"), c.Param("project_id"), in, meta(c))
		reply(c, v, e)
	})
	for _, op := range []access.Operation{access.OpProjectCreateCandidates, access.OpProjectRecoveryCandidates, access.OpProjectMemberCandidates, access.OpProjectOwnerCandidates, access.OpProjectAdministratorCandidates} {
		policy, _ := access.PolicyFor(op)
		r.GET(policy.Path, query("q"), func(c *gin.Context) {
			v, e := s.Candidates(c.Request.Context(), actor(c), c.Param("tenant_id"), c.Param("project_id"), op, c.Query("q"))
			reply(c, v, e)
		})
	}
	for _, path := range []string{p + "/members", p + "/roles/:role_id/members"} {
		r.GET(path, query("q", "status", "cursor", "limit"), func(c *gin.Context) {
			v, e := s.Members(c.Request.Context(), actor(c), c.Param("tenant_id"), c.Param("project_id"), c.Param("role_id"), filter(c))
			reply(c, v, e)
		})
	}
	r.GET(p+"/members/:membership_id", query(), func(c *gin.Context) {
		v, e := s.Member(c.Request.Context(), actor(c), c.Param("tenant_id"), c.Param("project_id"), c.Param("membership_id"))
		reply(c, v, e)
	})
	r.POST(p+"/members", query(), func(c *gin.Context) {
		in, ok := decode[projects.AddMemberInput](c)
		if !ok {
			return
		}
		v, e := s.AddMember(c.Request.Context(), actor(c), c.Param("tenant_id"), c.Param("project_id"), in, meta(c))
		reply(c, v, e)
	})
	r.GET(p+"/members/:membership_id/authorization", query(), func(c *gin.Context) {
		v, e := s.Authorization(c.Request.Context(), actor(c), c.Param("tenant_id"), c.Param("project_id"), c.Param("membership_id"))
		reply(c, v, e)
	})
	r.GET(p+"/permissions", query(), func(c *gin.Context) {
		v, e := s.Permissions(c.Request.Context(), actor(c), c.Param("tenant_id"), c.Param("project_id"))
		reply(c, v, e)
	})
	r.GET(p+"/audit", query("q", "cursor", "limit", "event_type", "from", "to", "object_id", "actor_id"), func(c *gin.Context) {
		v, e := s.Audit(c.Request.Context(), actor(c), c.Param("tenant_id"), c.Param("project_id"), filter(c))
		reply(c, v, e)
	})
	for _, presets := range []bool{false, true} {
		path := p + "/roles"
		if presets {
			path = "/api/v1/access/project-role-presets"
		}
		r.GET(path, query("q", "cursor", "limit", "status"), func(c *gin.Context) {
			v, e := s.Roles(c.Request.Context(), actor(c), c.Param("tenant_id"), c.Param("project_id"), filter(c), presets)
			reply(c, v, e)
		})
		r.GET(path+"/:role_id", query(), func(c *gin.Context) {
			v, e := s.Role(c.Request.Context(), actor(c), c.Param("tenant_id"), c.Param("project_id"), c.Param("role_id"), presets)
			reply(c, v, e)
		})
	}
	for _, path := range []string{p + "/roles", p + "/roles/:role_id/copy"} {
		r.POST(path, query(), func(c *gin.Context) {
			in, ok := decode[projects.RoleInput](c)
			if !ok {
				return
			}
			v, e := s.CreateRole(c.Request.Context(), actor(c), c.Param("tenant_id"), c.Param("project_id"), c.Param("role_id"), in, meta(c))
			reply(c, v, e)
		})
	}
	r.PATCH(p+"/roles/:role_id", query(), func(c *gin.Context) {
		in, ok := decode[struct {
			projects.ChangeMeta
			Name        string `json:"name"`
			Description string `json:"description"`
		}](c)
		if !ok {
			return
		}
		v, e := s.UpdateRole(c.Request.Context(), actor(c), c.Param("tenant_id"), c.Param("project_id"), c.Param("role_id"), projects.RoleInput{ChangeMeta: in.ChangeMeta, Name: in.Name, Description: in.Description}, meta(c))
		reply(c, v, e)
	})
	// Every action is an explicit registered policy, never an arbitrary action from a URL/body.
	for _, op := range []access.Operation{access.OpProjectArchive, access.OpProjectRestore, access.OpProjectOwnerChange, access.OpProjectTransfer, access.OpProjectRecoverAdmin, access.OpProjectMemberSuspend, access.OpProjectMemberResume, access.OpProjectMemberRemove, access.OpProjectMemberRoles, access.OpProjectRolePermissions, access.OpProjectRoleStatus, access.OpProjectRoleDelete} {
		for _, preview := range []bool{false, true} {
			pol, _ := access.PolicyFor(op)
			path := pol.Path
			method := pol.Method
			if preview {
				path += "/preview"
				method = "POST"
			}
			r.Handle(method, path, query(), func(c *gin.Context) {
				var in projects.Change
				ok := false
				switch op {
				case access.OpProjectOwnerChange, access.OpProjectTransfer, access.OpProjectRecoverAdmin:
					v, valid := decode[projects.CandidateInput](c)
					ok = valid
					in.ChangeMeta = v.ChangeMeta
					in.Candidate = v.Candidate
				case access.OpProjectRolePermissions:
					v, valid := decode[projects.PermissionsInput](c)
					ok = valid
					in.ChangeMeta = v.ChangeMeta
					in.Permissions = v.Permissions
				case access.OpProjectMemberRoles:
					v, valid := decode[projects.RolesInput](c)
					ok = valid
					in.ChangeMeta = v.ChangeMeta
					in.Roles = v.Roles
					in.RoleVersions = v.RoleVersions
				case access.OpProjectRoleStatus:
					v, valid := decode[projects.StatusInput](c)
					ok = valid
					in.ChangeMeta = v.ChangeMeta
					in.Status = v.Status
				default:
					v, valid := decode[projects.ChangeMeta](c)
					ok = valid
					in.ChangeMeta = v
				}
				if !ok {
					return
				}
				target := c.Param("membership_id")
				if target == "" {
					target = c.Param("role_id")
				}
				if preview {
					v, e := s.Preview(c.Request.Context(), actor(c), c.Param("tenant_id"), c.Param("project_id"), target, op, in)
					reply(c, v, e)
				} else {
					v, e := s.Change(c.Request.Context(), actor(c), c.Param("tenant_id"), c.Param("project_id"), target, op, in, meta(c))
					reply(c, v, e)
				}
			})
		}
	}
	for _, route := range r.Routes() {
		if strings.Contains(route.Path, "/projects") || strings.Contains(route.Path, "/project-") {
			if _, ok := access.RoutePolicy(route.Method, route.Path); !ok {
				panic("project policy missing")
			}
		}
	}
}
