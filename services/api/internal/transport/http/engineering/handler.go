package engineeringhttp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	identityhttp "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/identity"
	"github.com/gin-gonic/gin"
)

func decode[T any](c *gin.Context) (T, bool) {
	var v *T
	d := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, engineering.MaxContentBytes+16384))
	d.DisallowUnknownFields()
	var extra any
	if d.Decode(&v) != nil || v == nil || d.Decode(&extra) != io.EOF {
		identityhttp.RespondError(c, access.Validation("request", "请求包含未知字段、无效内容或超过大小限制。"))
		var zero T
		return zero, false
	}
	return *v, true
}
func reply(c *gin.Context, v any, e error) {
	if e != nil {
		var domain *engineering.Error
		if errors.As(e, &domain) {
			c.AbortWithStatusJSON(domain.Status, gin.H{"code": domain.Code, "message": domain.Message, "issues": domain.Issues, "request_id": c.Writer.Header().Get("X-Request-ID")})
			return
		}
		identityhttp.RespondError(c, e)
		return
	}
	c.JSON(200, v)
}
func query(allowed ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		for k, vs := range c.Request.URL.Query() {
			if !slices.Contains(allowed, k) || len(vs) != 1 {
				identityhttp.RespondError(c, access.Validation(k, "不支持或重复的查询参数。"))
				return
			}
		}
		c.Next()
	}
}
func integer(c *gin.Context, key string) int {
	v := c.Query(key)
	if v == "" {
		return 0
	}
	n, e := strconv.Atoi(v)
	if e != nil {
		return -1
	}
	return n
}
func Register(r *gin.Engine, i *identity.Service, s *engineering.Service) {
	actor := func(c *gin.Context) access.Actor { return i.AccessActor(identityhttp.Principal(c)) }
	meta := func(c *gin.Context) access.Meta {
		return access.Meta{ActorType: "account", IP: c.ClientIP(), UserAgent: c.Request.UserAgent(), RequestID: c.Writer.Header().Get("X-Request-ID")}
	}
	for _, p := range access.Policies() {
		if strings.HasPrefix(string(p.Operation), "Device:") || !strings.Contains(p.Path, "/engineering/") || strings.HasPrefix(string(p.Operation), "Resource:") || strings.HasPrefix(string(p.Operation), "ProjectResource:") || strings.HasPrefix(string(p.Operation), "ProjectResourceAccess:") {
			continue
		}
		parts := strings.Split(string(p.Operation), ":")
		action := parts[len(parts)-1]
		category := strings.HasPrefix(string(p.Operation), "EngineeringCategories:")
		allowed := []string{}
		if action == "list" && !category {
			allowed = []string{"q", "status", "offset", "limit"}
		}
		if action == "diff" {
			allowed = []string{"from", "to", "revision"}
		}
		r.Handle(p.Method, p.Path, query(allowed...), func(c *gin.Context) {
			a := actor(c)
			m := meta(c)
			ctx := c.Request.Context()
			tid := c.Param("tenant_id")
			if category {
				switch action {
				case "list", "catalog":
					v, e := s.Categories(ctx, a, tid)
					reply(c, v, e)
				case "save":
					in, ok := decode[engineering.CategoryInput](c)
					if ok {
						v, e := s.SaveCategory(ctx, a, in, m)
						reply(c, v, e)
					}
				case "delete":
					in, ok := decode[engineering.RevisionInput](c)
					if ok {
						v, e := s.DeleteCategory(ctx, a, c.Param("category_key"), in, m)
						reply(c, v, e)
					}
				}
				return
			}
			scope := engineering.Scope{Tenant: tid, Catalog: strings.Contains(p.Path, "/catalog/"), Kind: engineering.Kind(parts[1])}
			id := c.Param("definition_id")
			switch action {
			case "receipt":
				v, e := s.Receipt(ctx, a, scope, c.Param("request_id"))
				reply(c, v, e)
			case "list":
				v, e := s.List(ctx, a, scope, engineering.Filter{Query: c.Query("q"), Status: c.Query("status"), Offset: integer(c, "offset"), Limit: integer(c, "limit")})
				reply(c, v, e)
			case "detail":
				v, e := s.Detail(ctx, a, scope, id)
				reply(c, v, e)
			case "diff":
				v, e := s.Diff(ctx, a, scope, id, c.Query("from"), c.Query("to"), integer(c, "revision"))
				reply(c, v, e)
			case "impact":
				v, e := s.Impact(ctx, a, scope, id, c.Param("version_id"))
				reply(c, v, e)
			case "create":
				in, ok := decode[engineering.CreateInput](c)
				if ok {
					v, e := s.Create(ctx, a, scope, in, m)
					reply(c, v, e)
				}
			case "save":
				in, ok := decode[engineering.SaveInput](c)
				if ok {
					v, e := s.Save(ctx, a, scope, id, in, m)
					reply(c, v, e)
				}
			case "new-draft":
				in, ok := decode[engineering.NewDraftInput](c)
				if ok {
					v, e := s.NewDraft(ctx, a, scope, id, in, m)
					reply(c, v, e)
				}
			case "delete-draft":
				in, ok := decode[engineering.RevisionInput](c)
				if ok {
					v, e := s.DeleteDraft(ctx, a, scope, id, in, m)
					reply(c, v, e)
				}
			case "copy":
				in, ok := decode[engineering.CopyInput](c)
				if ok {
					v, e := s.Copy(ctx, a, scope, in, m)
					reply(c, v, e)
				}
			case "publish":
				in, ok := decode[engineering.RevisionInput](c)
				if ok {
					v, e := s.Publish(ctx, a, scope, id, in, m)
					reply(c, v, e)
				}
			case "state":
				in, ok := decode[engineering.StateInput](c)
				if ok {
					v, e := s.SetState(ctx, a, scope, id, c.Param("version_id"), in, m)
					reply(c, v, e)
				}
			case "preview":
				in, ok := decode[engineering.PreviewInput](c)
				if ok {
					v, e := s.Preview(ctx, a, scope, id, in)
					if e == nil && !v.Valid {
						e = &engineering.Error{Code: "definition_invalid", Message: "请修正标记的问题后重试。", Status: 422, Issues: v.Issues}
					}
					reply(c, v, e)
				}
			}
		})
	}
}
