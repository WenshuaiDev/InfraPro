package audithttp

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/WenshuaiDev/InfraPro/services/api/internal/routes"
	"io"
	"mime"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/audit"
	ar "github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	fileshttp "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/files"
	identityhttp "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/identity"
	"github.com/gin-gonic/gin"
)

func reply(c *gin.Context, status int, v any, e error) {
	if e != nil {
		access.ReportOperationFailure(c.Request.Context(), e)
		var problem *audit.Error
		if errors.As(e, &problem) {
			if problem.Status == 429 || problem.Status == 503 {
				c.Header("Retry-After", "2")
			}
			c.JSON(problem.Status, gin.H{"code": problem.Code, "message": problem.Message, "request_id": c.Writer.Header().Get("X-Request-ID"), "retryable": problem.Retryable, "next_action": problem.NextAction})
			return
		}
		identityhttp.RespondError(c, e)
		return
	}
	c.JSON(status, v)
}
func query(c *gin.Context, allowed ...string) error {
	for key, values := range c.Request.URL.Query() {
		if !slices.Contains(allowed, key) || len(values) != 1 {
			return audit.Problem("audit_filter_invalid", 400)
		}
	}
	return nil
}

var filters = []string{"reference_id", "from", "to", "actor_id", "object_type", "object_id", "event_type", "outcome", "category", "operation_id", "request_id", "q", "task_module", "task_kind", "task_id", "generation", "cursor", "limit"}

func filter(c *gin.Context) audit.Filter {
	limit := 0
	if c.Query("limit") != "" {
		var e error
		limit, e = strconv.Atoi(c.Query("limit"))
		if e != nil {
			limit = -1
		}
	}
	return audit.Filter{ReferenceID: c.Query("reference_id"), From: c.Query("from"), To: c.Query("to"), ActorID: c.Query("actor_id"), ObjectType: c.Query("object_type"), ObjectID: c.Query("object_id"), EventType: c.Query("event_type"), Outcome: c.Query("outcome"), Category: c.Query("category"), OperationID: c.Query("operation_id"), RequestID: c.Query("request_id"), Query: c.Query("q"), TaskModule: c.Query("task_module"), TaskKind: c.Query("task_kind"), TaskID: c.Query("task_id"), Generation: c.Query("generation"), Cursor: c.Query("cursor"), Limit: limit}
}
func uniqueJSON(d *json.Decoder, depth int) error {
	if depth > 16 {
		return audit.Problem("audit_filter_invalid", 400)
	}
	token, e := d.Token()
	if e != nil {
		return e
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			key, e := d.Token()
			if e != nil {
				return e
			}
			k, ok := key.(string)
			if !ok || seen[strings.ToLower(k)] {
				return audit.Problem("audit_filter_invalid", 400)
			}
			seen[strings.ToLower(k)] = true
			if e = uniqueJSON(d, depth+1); e != nil {
				return e
			}
		}
	case '[':
		for d.More() {
			if e = uniqueJSON(d, depth+1); e != nil {
				return e
			}
		}
	default:
		return audit.Problem("audit_filter_invalid", 400)
	}
	_, e = d.Token()
	return e
}
func decode(c *gin.Context, out *audit.ExportInput) error {
	typ, _, e := mime.ParseMediaType(c.ContentType())
	if e != nil || typ != "application/json" {
		return audit.Problem("audit_filter_invalid", 415)
	}
	raw, e := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10))
	if e != nil {
		return audit.Problem("audit_filter_invalid", 413)
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if e = uniqueJSON(d, 0); e != nil {
		return audit.Problem("audit_filter_invalid", 400)
	}
	if _, e = d.Token(); e != io.EOF {
		return audit.Problem("audit_filter_invalid", 400)
	}
	var fields map[string]json.RawMessage
	if e = json.Unmarshal(raw, &fields); e != nil || fields == nil {
		return audit.Problem("audit_filter_invalid", 400)
	}
	for key := range fields {
		if !slices.Contains([]string{"filter", "field_profile", "reason", "epoch", "authorization_revision", "projection_version"}, key) {
			return audit.Problem("audit_filter_invalid", 400)
		}
	}
	if rawFilter, ok := fields["filter"]; ok {
		var values map[string]json.RawMessage
		if json.Unmarshal(rawFilter, &values) != nil || values == nil {
			return audit.Problem("audit_filter_invalid", 400)
		}
		for key := range values {
			if !slices.Contains(filters, key) || key == "cursor" || key == "limit" {
				return audit.Problem("audit_filter_invalid", 400)
			}
		}
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if e = d.Decode(out); e != nil {
		return audit.Problem("audit_filter_invalid", 400)
	}
	return nil
}
func Register(r routes.Registrar, i *identity.Service, s *audit.Service) {
	for _, policy := range access.Policies() {
		parts := strings.SplitN(string(policy.Operation), ":", 3)
		if len(parts) != 3 || parts[0] != "Audit" {
			continue
		}
		kind, action := parts[1], parts[2]
		r.Handle(policy.Method, policy.Path, func(c *gin.Context) {
			c.Header("Cache-Control", "private, no-store")
			scope := audit.Scope{Kind: kind, Tenant: c.Param("tenant_id"), Project: c.Param("project_id")}
			if !scope.Valid() {
				reply(c, 0, nil, audit.Problem("audit_not_found", 404))
				return
			}
			for _, p := range c.Params {
				valid := ar.ID(p.Value)
				if p.Key == "event_id" || p.Key == "generation" {
					valid = ar.Number(p.Value, 1)
				}
				if !valid {
					reply(c, 0, nil, audit.Problem("audit_reference_invalid", 400))
					return
				}
			}
			a := i.AccessActor(identityhttp.Principal(c))
			ctx := c.Request.Context()
			var v any
			var e error
			status := 200
			switch action {
			case "capabilities":
				if e = query(c); e == nil {
					v, e = s.Capabilities(ctx, a, scope)
				}
			case "events":
				if e = query(c, filters...); e == nil {
					v, e = s.Events(ctx, a, scope, filter(c))
				}
			case "event":
				if e = query(c); e == nil {
					v, e = s.Event(ctx, a, scope, c.Param("event_id"))
				}
			case "trace":
				if e = query(c, "cursor", "limit"); e == nil {
					f := filter(c)
					v, e = s.Trace(ctx, a, scope, c.Param("event_id"), audit.TraceFilter{Cursor: f.Cursor, Limit: f.Limit})
				}
			case "target":
				if e = query(c); e == nil {
					v, e = s.Target(ctx, a, scope, c.Param("event_id"), c.Param("link_id"))
				}
			case "exports.create":
				status = 202
				if e = query(c); e == nil {
					var input audit.ExportInput
					if e = decode(c, &input); e == nil {
						command := c.GetHeader("X-Command-ID")
						if len(c.Request.Header.Values("X-Command-ID")) > 1 {
							e = audit.Problem("audit_filter_invalid", 400)
						} else {
							v, e = s.CreateExport(ctx, a, scope, command, input)
						}
					}
				}
			case "exports":
				if e = query(c, "cursor", "limit", "mode", "state", "from", "to"); e == nil {
					mode := c.Query("mode")
					if mode != "" && mode != "mine" && mode != "managed" {
						e = audit.Problem("audit_filter_invalid", 400)
					} else {
						v, e = s.Exports(ctx, a, scope, audit.ExportFilter{From: c.Query("from"), To: c.Query("to"), State: c.Query("state"), Cursor: c.Query("cursor"), Limit: filter(c).Limit}, mode == "managed")
					}
				}
			case "export":
				if e = query(c, "generation"); e == nil {
					v, e = s.Export(ctx, a, scope, c.Param("export_id"), c.Query("generation"))
				}
			case "download", "download.head":
				if e = query(c); e == nil {
					g, err := s.OpenRead(ctx, a, scope, c.Param("export_id"), c.Param("generation"), c.Request.Method, c.Writer.Header().Get("X-Request-ID"))
					if err != nil {
						reply(c, 0, nil, err)
						return
					}
					defer g.Close()
					serveRead(c, g)
					return
				}
			default:
				e = access.ErrDenied
			}
			reply(c, status, v, e)
		})
	}
}
func serveRead(c *gin.Context, g *audit.GuardedRead) {
	controller := fileshttp.ResponseController(c)
	if controller == nil {
		reply(c, 0, nil, audit.Problem("audit_transport_unavailable", 503))
		return
	}
	if e := controller.SetWriteDeadline(time.Now().Add(2 * time.Second)); e != nil {
		reply(c, 0, nil, audit.Problem("audit_transport_unavailable", 503))
		return
	}
	defer controller.SetWriteDeadline(time.Time{})
	etag := "\"" + g.SHA256 + "\""
	raw := c.GetHeader("Range")
	status := 200
	if v := c.GetHeader("If-Match"); v != "" && !fileshttp.MatchesETag(v, etag, false) {
		status = 412
	}
	if v := c.GetHeader("If-None-Match"); status == 200 && v != "" && fileshttp.MatchesETag(v, etag, true) {
		status = 304
	}
	if v := c.GetHeader("If-Range"); v != "" && v != etag {
		raw = ""
	}
	offset, length, e := fileshttp.SingleRange(raw, g.Size)
	if e != nil || len(c.Request.Header.Values("Range")) > 1 {
		if status == 200 {
			status = 416
		}
	} else if raw != "" && status == 200 {
		status = 206
	}
	e = g.Respond(c.Request.Context(), func() error {
		c.Header("ETag", etag)
		c.Header("Cache-Control", "private, no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Accept-Ranges", "bytes")
		if status == 416 {
			c.Header("Content-Range", "bytes */"+strconv.FormatInt(g.Size, 10))
		} else if status != 304 && status != 412 {
			c.Header("Content-Type", g.MediaType)
			c.Header("Content-Disposition", `attachment; filename="audit.csv"; filename*=UTF-8''%E5%AE%A1%E8%AE%A1%E5%AF%BC%E5%87%BA.csv`)
			c.Header("Content-Length", strconv.FormatInt(length, 10))
			if status == 206 {
				c.Header("Content-Range", "bytes "+strconv.FormatInt(offset, 10)+"-"+strconv.FormatInt(offset+length-1, 10)+"/"+strconv.FormatInt(g.Size, 10))
			}
		}
		c.Status(status)
		c.Writer.WriteHeaderNow()
		return nil
	})
	if e != nil {
		reply(c, 0, nil, e)
		return
	}
	if status == 304 || status == 412 || status == 416 || c.Request.Method == "HEAD" {
		return
	}
	e = g.Write(c.Request.Context(), offset, length, func(b []byte) (int, error) {
		if e := controller.SetWriteDeadline(time.Now().Add(2 * time.Second)); e != nil {
			return 0, e
		}
		n, e := c.Writer.Write(b)
		if e == nil {
			e = controller.Flush()
		}
		return n, e
	})
	if e != nil {
		c.Abort()
	}
}
