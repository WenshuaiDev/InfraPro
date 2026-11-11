package resourceshttp

import (
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
func Register(r *gin.Engine, i *identity.Service, s *resources.Service) {
	for _, p := range access.Policies() {
		if !strings.HasPrefix(string(p.Operation), "Resource:") {
			continue
		}
		parts := strings.Split(string(p.Operation), ":")
		kind, action := parts[1], parts[2]
		r.Handle(p.Method, p.Path, func(c *gin.Context) {
			allowed := []string{}
			if action == "list" {
				allowed = []string{"q", "status", "cursor", "limit"}
				switch kind {
				case "facility":
					allowed = append(allowed, "type_definition_id", "deleted")
				case "component":
					allowed = append(allowed, "parent_id")
				case "point":
					allowed = append(allowed, "owner")
				}
			}
			if action == "changes" {
				allowed = []string{"cursor", "limit", "kind", "id"}
			}
			for key, vs := range c.Request.URL.Query() {
				if !slices.Contains(allowed, key) || len(vs) != 1 {
					reply(c, 0, nil, &resources.Error{Code: "invalid_request", Message: "不支持或重复的查询参数。", Status: 400})
					return
				}
			}
			a := i.AccessActor(identityhttp.Principal(c))
			ctx := c.Request.Context()
			t := resources.Target{Tenant: c.Param("tenant_id"), Facility: c.Param("facility_id"), Point: c.Param("point_id"), Kind: resources.Kind(kind), ID: c.Param(kind + "_id")}
			if kind == "request" {
				v, e := s.Receipt(ctx, a, t.Tenant, c.Param("request_id"))
				reply(c, 200, v, e)
				return
			}
			limit := 0
			if q := c.Query("limit"); q != "" {
				var e error
				limit, e = strconv.Atoi(q)
				if e != nil || limit < 1 {
					reply(c, 0, nil, &resources.Error{Code: "invalid_request", Message: "分页大小无效。", Status: 400})
					return
				}
			}
			f := resources.Filter{Query: c.Query("q"), Status: c.Query("status"), TypeDefinition: c.Query("type_definition_id"), Deleted: c.Query("deleted"), Parent: c.Query("parent_id"), Owner: c.Query("owner"), Cursor: c.Query("cursor"), Limit: limit, Kind: c.Query("kind"), ID: c.Query("id")}
			switch action {
			case "list":
				v, e := s.List(ctx, a, t, f)
				reply(c, 200, v, e)
			case "detail":
				v, e := s.Detail(ctx, a, t)
				reply(c, 200, v, e)
			case "changes":
				v, e := s.Changes(ctx, a, t, f)
				reply(c, 200, v, e)
			default:
				raw, e := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 256*1024))
				if e != nil {
					var large *http.MaxBytesError
					if errors.As(e, &large) {
						reply(c, 0, nil, &resources.Error{Code: "input_too_large", Message: "单次请求最多 256 KiB。", Status: 413})
					} else {
						reply(c, 0, nil, &resources.Error{Code: "invalid_request", Message: "请求读取失败。", Status: 400})
					}
					return
				}
				m := access.Meta{ActorType: "account", IP: c.ClientIP(), UserAgent: c.Request.UserAgent(), RequestID: c.Writer.Header().Get("X-Request-ID")}
				v, e := s.Mutate(ctx, a, t, action, json.RawMessage(raw), m)
				status := 200
				if action == "create" {
					status = 201
				}
				reply(c, status, v, e)
			}
		})
	}
}
