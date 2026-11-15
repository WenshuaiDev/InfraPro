package taskshttp

import (
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/tasks"
	identityhttp "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/identity"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"slices"
	"strconv"
)

func query(keys ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		for k, v := range c.Request.URL.Query() {
			if !slices.Contains(keys, k) || len(v) != 1 {
				identityhttp.RespondError(c, access.Validation("query", "不支持的任务查询参数。"))
				c.Abort()
				return
			}
		}
		c.Next()
	}
}
func filter(c *gin.Context) tasks.Filter {
	limit := 0
	if c.Query("limit") != "" {
		var e error
		limit, e = strconv.Atoi(c.Query("limit"))
		if e != nil {
			limit = -1
		}
	}
	return tasks.Filter{ScopeKind: c.Query("scope_kind"), Source: c.Query("source"), Mode: c.Query("mode"), Tenant: c.Query("tenant_id"), Project: c.Query("project_id"), Type: c.Query("type"), Group: c.Query("group"), From: c.Query("from"), To: c.Query("to"), Q: c.Query("q"), Sort: c.Query("sort"), Cursor: c.Query("cursor"), Limit: limit}
}
func reply(c *gin.Context, v any, e error) {
	if e != nil {
		identityhttp.RespondError(c, e)
		return
	}
	switch out := v.(type) {
	case tasks.Page:
		c.Header("X-Task-Authorization-Revision", out.AuthorizationRevision)
	case tasks.Summary:
		c.Header("X-Task-Authorization-Revision", out.AuthorizationRevision)
	}
	status := 200
	if r, ok := v.(tasks.CommandResult); ok && r.CommandState == "requested" {
		status = 202
	}
	c.JSON(status, v)
}
func Register(r *gin.Engine, i *identity.Service, s *tasks.Service) {
	actor := func(c *gin.Context) access.Actor { return i.AccessActor(identityhttp.Principal(c)) }
	filters := []string{"scope_kind", "source", "mode", "tenant_id", "project_id", "type", "group", "from", "to", "q", "sort", "cursor", "limit"}
	r.GET("/api/v1/tasks", query(filters...), func(c *gin.Context) { v, e := s.List(c.Request.Context(), actor(c), filter(c)); reply(c, v, e) })
	r.GET("/api/v1/tasks/summary", query(filters...), func(c *gin.Context) { v, e := s.Summary(c.Request.Context(), actor(c), filter(c)); reply(c, v, e) })
	r.GET("/api/v1/tasks/catalog", query(filters...), func(c *gin.Context) {
		v, e := s.Catalog(c.Request.Context(), actor(c), filter(c))
		reply(c, gin.H{"items": v}, e)
	})
	r.GET("/api/v1/tasks/resolve", query("module", "kind", "source_id"), func(c *gin.Context) {
		v, e := s.Resolve(c.Request.Context(), actor(c), taskrecord.SourceRef{Module: c.Query("module"), Kind: c.Query("kind"), RootID: c.Query("source_id")})
		reply(c, v, e)
	})
	r.GET("/api/v1/tasks/operations", query(filters...), func(c *gin.Context) {
		v, e := s.Operations(c.Request.Context(), actor(c), filter(c))
		reply(c, gin.H{"items": v}, e)
	})
	r.GET("/api/v1/tasks/:id", query("generation"), func(c *gin.Context) {
		v, e := s.Get(c.Request.Context(), actor(c), c.Param("id"), c.Query("generation"))
		reply(c, v, e)
	})
	r.GET("/api/v1/tasks/:id/executions", query("cursor"), func(c *gin.Context) {
		v, e := s.Executions(c.Request.Context(), actor(c), c.Param("id"), c.Query("cursor"))
		reply(c, v, e)
	})
	r.GET("/api/v1/tasks/:id/events", query("generation", "cursor"), func(c *gin.Context) {
		v, e := s.Events(c.Request.Context(), actor(c), c.Param("id"), c.Query("generation"), c.Query("cursor"))
		reply(c, v, e)
	})
	r.GET("/api/v1/tasks/:id/target", query("generation", "target_kind"), func(c *gin.Context) {
		kind := c.DefaultQuery("target_kind", "business")
		v, e := s.Target(c.Request.Context(), actor(c), c.Param("id"), c.Query("generation"), kind)
		reply(c, v, e)
	})
	r.POST("/api/v1/tasks/:id/commands", query(), func(c *gin.Context) {
		var in tasks.Command
		decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 8192))
		decoder.DisallowUnknownFields()
		var rest any
		if decoder.Decode(&in) != nil || decoder.Decode(&rest) != io.EOF {
			identityhttp.RespondError(c, access.Validation("command", "任务命令无效或过大。"))
			return
		}
		v, e := s.Command(c.Request.Context(), actor(c), c.Param("id"), in)
		reply(c, v, e)
	})
}
