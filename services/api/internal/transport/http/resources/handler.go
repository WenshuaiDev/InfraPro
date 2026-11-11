package resourceshttp

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	identityhttp "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/identity"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
)

func reply(c *gin.Context, status int, v any, e error) {
	if e != nil {
		var domain *resources.Error
		if errors.As(e, &domain) {
			if domain.Issues == nil {
				domain.Issues = []engineering.Issue{}
			}
			c.AbortWithStatusJSON(domain.Status, domain)
			return
		}
		identityhttp.RespondError(c, e)
		return
	}
	c.JSON(status, v)
}
func invalid(message string) error {
	return &resources.Error{Code: "invalid_request", Message: message, Status: 400}
}
func body(c *gin.Context) ([]byte, error) {
	raw, e := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 256*1024))
	if e != nil {
		var large *http.MaxBytesError
		if errors.As(e, &large) {
			return nil, &resources.Error{Code: "input_too_large", Message: "单次请求最多 256 KiB。", Status: 413}
		}
		return nil, invalid("请求读取失败。")
	}
	return raw, nil
}
func decode(raw []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return invalid("请求字段或类型无效。")
	}
	if e := d.Decode(new(any)); e != io.EOF {
		return invalid("请提交单个对象。")
	}
	return nil
}
func query(c *gin.Context, allowed []string) error {
	for key, values := range c.Request.URL.Query() {
		if !slices.Contains(allowed, key) || len(values) != 1 {
			return invalid("不支持或重复的查询参数。")
		}
	}
	return nil
}
func number(c *gin.Context, key string) (int, error) {
	if c.Query(key) == "" {
		return 0, nil
	}
	n, e := strconv.Atoi(c.Query(key))
	if e != nil || n < 0 || key != "offset" && n == 0 {
		return 0, invalid("分页参数无效。")
	}
	return n, nil
}
func requestMeta(c *gin.Context) access.Meta {
	return access.Meta{ActorType: "account", IP: c.ClientIP(), UserAgent: c.Request.UserAgent(), RequestID: c.Writer.Header().Get("X-Request-ID")}
}
func Register(r *gin.Engine, i *identity.Service, s *resources.Service) {
	for _, p := range access.Policies() {
		parts := strings.Split(string(p.Operation), ":")
		if len(parts) != 3 || !slices.Contains([]string{"Resource", "ProjectResource", "ProjectResourceAccess"}, parts[0]) {
			continue
		}
		namespace, kind, action := parts[0], parts[1], parts[2]
		r.Handle(p.Method, p.Path, func(c *gin.Context) {
			if namespace == "ProjectResourceAccess" {
				accessHandler(c, i, s, kind, action)
				return
			}
			a := i.AccessActor(identityhttp.Principal(c))
			ctx := c.Request.Context()
			scope := resources.TenantScope(c.Param("tenant_id"))
			if namespace == "ProjectResource" {
				scope = resources.ProjectScope(c.Param("tenant_id"), c.Param("project_id"))
			}
			if slices.Contains([]string{"type-definitions", "metrics", "units"}, kind) {
				allowed := []string{}
				if action == "list" {
					allowed = []string{"q", "limit", "offset"}
				}
				if e := query(c, allowed); e != nil {
					reply(c, 0, nil, e)
					return
				}
				if action == "version" {
					v, e := s.CatalogVersion(ctx, a, scope, engineering.Kind(kind), c.Param("version_id"))
					reply(c, 200, v, e)
					return
				}
				limit, e := number(c, "limit")
				if e != nil {
					reply(c, 0, nil, e)
					return
				}
				offset, e := number(c, "offset")
				if e != nil {
					reply(c, 0, nil, e)
					return
				}
				v, e := s.CatalogList(ctx, a, scope, engineering.Kind(kind), engineering.Filter{Query: c.Query("q"), Limit: limit, Offset: offset})
				reply(c, 200, v, e)
				return
			}
			allowed := []string{}
			if action == "list" {
				allowed = []string{"q", "status", "cursor", "limit"}
				switch kind {
				case "facility":
					allowed = append(allowed, "type_definition_id")
					if namespace == "Resource" {
						allowed = append(allowed, "deleted")
					}
				case "component":
					allowed = append(allowed, "parent_id")
				case "point":
					allowed = append(allowed, "owner")
				}
			}
			if action == "changes" {
				allowed = []string{"cursor", "limit", "kind", "id"}
			}
			if e := query(c, allowed); e != nil {
				reply(c, 0, nil, e)
				return
			}
			t := resources.Target{Facility: c.Param("facility_id"), Point: c.Param("point_id"), Kind: resources.Kind(kind), ID: c.Param(kind + "_id")}
			if kind == "request" {
				v, e := s.Receipt(ctx, a, scope, c.Param("request_id"))
				reply(c, 200, v, e)
				return
			}
			limit, e := number(c, "limit")
			if e != nil {
				reply(c, 0, nil, e)
				return
			}
			f := resources.Filter{Query: c.Query("q"), Status: c.Query("status"), TypeDefinition: c.Query("type_definition_id"), Deleted: c.Query("deleted"), Parent: c.Query("parent_id"), Owner: c.Query("owner"), Cursor: c.Query("cursor"), Limit: limit, Kind: c.Query("kind"), ID: c.Query("id")}
			switch action {
			case "list":
				v, e := s.List(ctx, a, scope, t, f)
				reply(c, 200, v, e)
			case "detail":
				v, e := s.Detail(ctx, a, scope, t)
				reply(c, 200, v, e)
			case "changes":
				v, e := s.Changes(ctx, a, scope, t, f)
				reply(c, 200, v, e)
			default:
				raw, e := body(c)
				if e != nil {
					reply(c, 0, nil, e)
					return
				}
				if action == "preview" {
					var fields map[string]json.RawMessage
					if e = decode(raw, &fields); e != nil || fields == nil {
						reply(c, 0, nil, invalid("请提交有效的预览对象。"))
						return
					}
					var intent string
					if json.Unmarshal(fields["action"], &intent) != nil || !slices.Contains([]string{"update", "retire", "restore", "delete"}, intent) {
						reply(c, 0, nil, invalid("预览动作无效。"))
						return
					}
					delete(fields, "action")
					raw, e = json.Marshal(fields)
					if e != nil {
						reply(c, 0, nil, e)
						return
					}
					v, e := s.PreviewMutation(ctx, a, scope, t, intent, raw)
					reply(c, 200, v, e)
					return
				}
				v, e := s.Mutate(ctx, a, scope, t, action, raw, requestMeta(c))
				status := 200
				if action == "create" {
					status = 201
				}
				reply(c, status, v, e)
			}
		})
	}
}
func accessHandler(c *gin.Context, i *identity.Service, s *resources.Service, kind, action string) {
	a := i.AccessActor(identityhttp.Principal(c))
	ctx := c.Request.Context()
	tenant, project, facility := c.Param("tenant_id"), c.Param("project_id"), c.Param("facility_id")
	allowed := []string{}
	if action == "detail" && kind != "request" || action == "changes" {
		allowed = []string{"cursor", "limit"}
	}
	if action == "detail" && kind == "facility" {
		allowed = append(allowed, "grant_cursor", "candidate_kind", "candidate_cursor", "candidate_limit", "candidate_q", "candidate_status", "candidate_parent_id", "candidate_owner")
	}
	if e := query(c, allowed); e != nil {
		reply(c, 0, nil, e)
		return
	}
	switch action {
	case "detail", "changes":
		if kind == "request" {
			v, e := s.AccessReceipt(ctx, a, tenant, project, c.Param("request_id"))
			reply(c, 200, v, e)
			return
		}
		limit, e := number(c, "limit")
		if e != nil {
			reply(c, 0, nil, e)
			return
		}
		if kind == "usage" {
			v, e := s.FacilityUsage(ctx, a, tenant, facility, resources.AccessFilter{Limit: limit, Cursor: c.Query("cursor")})
			reply(c, 200, v, e)
			return
		}
		candidateLimit, e := number(c, "candidate_limit")
		if e != nil {
			reply(c, 0, nil, e)
			return
		}
		f := resources.AccessFilter{Limit: limit, Cursor: c.Query("cursor"), GrantCursor: c.Query("grant_cursor"), CandidateKind: resources.Kind(c.Query("candidate_kind")), Candidate: resources.Filter{Limit: candidateLimit, Cursor: c.Query("candidate_cursor"), Query: c.Query("candidate_q"), Status: c.Query("candidate_status"), Parent: c.Query("candidate_parent_id"), Owner: c.Query("candidate_owner")}}
		if action == "changes" {
			v, e := s.AccessChanges(ctx, a, tenant, project, f)
			reply(c, 200, v, e)
		} else {
			v, e := s.AccessDetail(ctx, a, tenant, project, facility, f)
			reply(c, 200, v, e)
		}
	default:
		raw, e := body(c)
		if e != nil {
			reply(c, 0, nil, e)
			return
		}
		var in resources.AccessInput
		if e = decode(raw, &in); e != nil {
			reply(c, 0, nil, e)
			return
		}
		if action == "preview" {
			v, e := s.PreviewAccessChange(ctx, a, tenant, project, facility, in)
			reply(c, 200, v, e)
		} else {
			v, e := s.ChangeAccess(ctx, a, tenant, project, facility, in, requestMeta(c))
			reply(c, 200, v, e)
		}
	}
}
