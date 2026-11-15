package fileshttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/files"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	identityhttp "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/identity"
	"github.com/gin-gonic/gin"
	"io"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

type transportKey struct{}

// WithTransport captures the actual server writer before framework wrappers so
// the mandatory deadline is enforced, never silently ignored as unsupported.
func WithTransport(r *http.Request, w http.ResponseWriter) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), transportKey{}, http.NewResponseController(w)))
}
func problem(code string, status int) error {
	return &files.Error{Code: code, Message: "文件请求无效，请核对后重试。", Status: status, NextAction: "none"}
}
func reply(c *gin.Context, status int, v any, e error) {
	if e != nil {
		access.ReportOperationFailure(c.Request.Context(), e)
		var fe *files.Error
		if errors.As(e, &fe) {
			if fe.Status == 429 || fe.Code == "part_in_progress" {
				c.Header("Retry-After", "2")
			}
			c.JSON(fe.Status, gin.H{"code": fe.Code, "message": fe.Message, "fields": fe.Fields, "request_id": c.Writer.Header().Get("X-Request-ID"), "retryable": fe.Retryable, "next_action": fe.NextAction})
			return
		}
		identityhttp.RespondError(c, e)
		return
	}
	c.JSON(status, v)
}
func query(c *gin.Context, allowed ...string) error {
	for k, v := range c.Request.URL.Query() {
		if !slices.Contains(allowed, k) || len(v) != 1 {
			return problem("invalid_request", 400)
		}
	}
	return nil
}
func limit(c *gin.Context) int {
	if c.Query("limit") == "" {
		return 0
	}
	v, e := strconv.Atoi(c.Query("limit"))
	if e != nil {
		return -1
	}
	return v
}
func timestamp(v string) (*time.Time, error) {
	if v == "" {
		return nil, nil
	}
	t, e := time.Parse(time.RFC3339Nano, v)
	if e != nil {
		return nil, problem("invalid_request", 400)
	}
	return &t, nil
}
func uniqueJSON(decoder *json.Decoder, depth int) error {
	if depth > 16 {
		return problem("invalid_request", 400)
	}
	token, e := decoder.Token()
	if e != nil {
		return e
	}
	d, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch d {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			k, e := decoder.Token()
			if e != nil {
				return e
			}
			key, ok := k.(string)
			if !ok || seen[strings.ToLower(key)] {
				return problem("invalid_request", 400)
			}
			seen[strings.ToLower(key)] = true
			if e = uniqueJSON(decoder, depth+1); e != nil {
				return e
			}
		}
	case '[':
		for decoder.More() {
			if e = uniqueJSON(decoder, depth+1); e != nil {
				return e
			}
		}
	default:
		return problem("invalid_request", 400)
	}
	_, e = decoder.Token()
	return e
}
func decode(c *gin.Context, out any, allowed ...string) error {
	typ, _, e := mime.ParseMediaType(c.ContentType())
	if e != nil || typ != "application/json" {
		return problem("invalid_request", 415)
	}
	raw, e := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 65536))
	if e != nil {
		return problem("invalid_request", 413)
	}
	probe := json.NewDecoder(bytes.NewReader(raw))
	if e = uniqueJSON(probe, 0); e != nil {
		return problem("invalid_request", 400)
	}
	if _, e = probe.Token(); e != io.EOF {
		return problem("invalid_request", 400)
	}
	var fields map[string]json.RawMessage
	if e = json.Unmarshal(raw, &fields); e != nil || fields == nil {
		return problem("invalid_request", 400)
	}
	allowed = append(allowed, "request_id", "reason")
	for key := range fields {
		if !slices.Contains(allowed, key) {
			return problem("invalid_request", 400)
		}
	}
	if host, ok := fields["host"]; ok {
		var h map[string]json.RawMessage
		if e = json.Unmarshal(host, &h); e != nil {
			return problem("invalid_request", 400)
		}
		for k := range h {
			if k != "kind" && k != "id" {
				return problem("invalid_request", 400)
			}
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if e = decoder.Decode(out); e != nil {
		return problem("invalid_request", 400)
	}
	return nil
}
func Register(r *gin.Engine, i *identity.Service, s *files.Service) {
	for _, policy := range access.Policies() {
		parts := strings.SplitN(string(policy.Operation), ":", 3)
		if len(parts) != 3 || parts[0] != "Files" {
			continue
		}
		kind, action := parts[1], parts[2]
		r.Handle(policy.Method, policy.Path, func(c *gin.Context) {
			c.Header("Cache-Control", "private, no-store")
			for _, p := range c.Params {
				if p.Key != "part" && !access.ValidID(p.Value) {
					reply(c, 0, nil, problem("file_not_found", 404))
					return
				}
			}
			scope := files.Scope{Kind: kind, TenantID: c.Param("tenant_id"), ProjectID: c.Param("project_id")}
			actor := i.AccessActor(identityhttp.Principal(c))
			ctx := c.Request.Context()
			id := c.Param("id")
			if c.Request.Method != "GET" && c.Request.Method != "HEAD" {
				if len(c.Request.Header.Values("X-Files-Epoch")) != 1 || !access.ValidID(c.GetHeader("X-Files-Epoch")) {
					reply(c, 0, nil, problem("file_epoch_changed", 409))
					return
				}
			}
			keys := []string{}
			switch action {
			case "attachments.list":
				keys = []string{"host_kind", "host_id", "state", "q", "created_from", "created_to", "cursor", "limit"}
			case "uploads.list":
				keys = []string{"view", "state", "cursor", "limit"}
			case "attachments.revisions":
				keys = []string{"cursor", "limit"}
			case "jobs.detail":
				keys = []string{"generation"}
			}
			if e := query(c, keys...); e != nil {
				reply(c, 0, nil, e)
				return
			}
			switch action {
			case "capabilities":
				v, e := s.Capabilities(ctx, actor, scope)
				reply(c, 200, v, e)
			case "attachments.list":
				from, e := timestamp(c.Query("created_from"))
				if e != nil {
					reply(c, 0, nil, e)
					return
				}
				to, e := timestamp(c.Query("created_to"))
				if e != nil {
					reply(c, 0, nil, e)
					return
				}
				v, e := s.Attachments(ctx, actor, scope, files.AttachmentFilter{Host: files.HostRef{Kind: c.Query("host_kind"), ID: c.Query("host_id")}, State: c.Query("state"), Q: c.Query("q"), CreatedFrom: from, CreatedTo: to, Cursor: c.Query("cursor"), Limit: limit(c)})
				reply(c, 200, v, e)
			case "attachments.detail":
				v, e := s.Attachment(ctx, actor, scope, id)
				reply(c, 200, v, e)
			case "attachments.revisions":
				v, e := s.Revisions(ctx, actor, scope, id, c.Query("cursor"), limit(c))
				reply(c, 200, v, e)
			case "attachments.update", "attachments.trash", "attachments.restore", "attachments.revert":
				var in files.AttachmentCommand
				keys := []string{"expected_revision"}
				if action == "attachments.update" {
					keys = append(keys, "display_name", "description")
				}
				if action == "attachments.revert" {
					keys = append(keys, "revision_id")
				}
				if e := decode(c, &in, keys...); e != nil {
					reply(c, 0, nil, e)
					return
				}
				in.Epoch = c.GetHeader("X-Files-Epoch")
				in.Action = strings.TrimPrefix(action, "attachments.")
				v, e := s.ChangeAttachment(ctx, actor, scope, id, in)
				reply(c, 200, v, e)
			case "uploads.list":
				v, e := s.Uploads(ctx, actor, scope, files.UploadFilter{View: c.Query("view"), State: c.Query("state"), Cursor: c.Query("cursor"), Limit: limit(c)})
				reply(c, 200, v, e)
			case "uploads.detail":
				v, e := s.Upload(ctx, actor, scope, id)
				reply(c, 200, v, e)
			case "uploads.create":
				var in files.StartUploadInput
				if e := decode(c, &in, "host", "intent", "attachment_id", "expected_attachment_revision", "filename", "display_name", "description", "size_bytes", "sha256", "declared_media_type"); e != nil {
					reply(c, 0, nil, e)
					return
				}
				in.Epoch = c.GetHeader("X-Files-Epoch")
				v, e := s.StartUpload(ctx, actor, scope, in)
				reply(c, 201, v, e)
			case "uploads.part":
				part, e := strconv.Atoi(c.Param("part"))
				if e != nil || len(c.Request.Header.Values("X-Content-SHA256")) != 1 || c.ContentType() != "application/octet-stream" || c.Request.ContentLength < 1 || c.Request.ContentLength > 8<<20 {
					reply(c, 0, nil, problem("invalid_request", 400))
					return
				}
				body := http.MaxBytesReader(c.Writer, c.Request.Body, 8<<20)
				v, e := s.PutPart(ctx, actor, scope, id, files.PartInput{Part: part, Size: c.Request.ContentLength, SHA256: c.GetHeader("X-Content-SHA256"), Epoch: c.GetHeader("X-Files-Epoch")}, body)
				reply(c, 200, v, e)
			case "uploads.complete", "uploads.cancel":
				var in files.RevisionCommand
				if e := decode(c, &in, "expected_revision"); e != nil {
					reply(c, 0, nil, e)
					return
				}
				in.Epoch = c.GetHeader("X-Files-Epoch")
				if action == "uploads.complete" {
					v, e := s.CompleteUpload(ctx, actor, scope, id, in)
					reply(c, 202, v, e)
				} else {
					v, e := s.CancelUpload(ctx, actor, scope, id, in)
					reply(c, 200, v, e)
				}
			case "uploads.reconfirm":
				var in files.ReconfirmInput
				if e := decode(c, &in, "expected_generation", "expected_control_revision", "expected_attachment_revision"); e != nil {
					reply(c, 0, nil, e)
					return
				}
				in.Epoch = c.GetHeader("X-Files-Epoch")
				v, e := s.Reconfirm(ctx, actor, scope, id, in)
				reply(c, 202, v, e)
			case "jobs.detail":
				v, e := s.Job(ctx, actor, scope, id, c.Query("generation"))
				reply(c, 200, v, e)
			case "requests.detail":
				v, e := s.Request(ctx, actor, scope, c.Param("request_id"))
				reply(c, 200, v, e)
			case "content", "content.head", "preview", "preview.head":
				variant := "source"
				if strings.HasPrefix(action, "preview") {
					variant = "preview"
				}
				v, e := s.OpenRead(ctx, actor, scope, id, c.Param("revision_id"), variant)
				if e != nil {
					reply(c, 0, nil, e)
					return
				}
				defer v.Close()
				serveRead(c, v)
			default:
				reply(c, 0, nil, access.ErrDenied)
			}
		})
	}
}
func singleRange(raw string, size int64) (offset, length int64, err error) {
	if raw == "" {
		return 0, size, nil
	}
	if !strings.HasPrefix(raw, "bytes=") || strings.Contains(raw, ",") {
		return 0, 0, problem("invalid_range", 416)
	}
	parts := strings.Split(strings.TrimPrefix(raw, "bytes="), "-")
	if len(parts) != 2 {
		return 0, 0, problem("invalid_range", 416)
	}
	parse := func(v string) (int64, error) {
		if v == "" || strings.IndexFunc(v, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			return 0, problem("invalid_range", 416)
		}
		n, e := strconv.ParseInt(v, 10, 64)
		return n, e
	}
	if parts[0] == "" {
		n, e := parse(parts[1])
		if e != nil || n <= 0 {
			return 0, 0, problem("invalid_range", 416)
		}
		n = min(n, size)
		return size - n, n, nil
	}
	start, e := parse(parts[0])
	if e != nil || start >= size {
		return 0, 0, problem("invalid_range", 416)
	}
	end := size - 1
	if parts[1] != "" {
		end, e = parse(parts[1])
		if e != nil || end < start {
			return 0, 0, problem("invalid_range", 416)
		}
		end = min(end, size-1)
	}
	return start, end - start + 1, nil
}
func matchesETag(raw, etag string, weak bool) bool {
	for _, v := range strings.Split(raw, ",") {
		v = strings.TrimSpace(v)
		if v == "*" || v == etag || weak && strings.TrimPrefix(v, "W/") == etag {
			return true
		}
	}
	return false
}
func serveRead(c *gin.Context, g *files.GuardedRead) {
	ctx := c.Request.Context()
	controller, ok := ctx.Value(transportKey{}).(*http.ResponseController)
	if !ok {
		reply(c, 0, nil, problem("file_transport_unavailable", 503))
		return
	}
	if e := controller.SetWriteDeadline(time.Now().Add(2 * time.Second)); e != nil {
		reply(c, 0, nil, problem("file_transport_unavailable", 503))
		return
	}
	defer controller.SetWriteDeadline(time.Time{})
	etag := "\"" + g.SHA256 + "\""
	rawRange := c.GetHeader("Range")
	status := 200
	if v := c.GetHeader("If-Match"); v != "" && !matchesETag(v, etag, false) {
		status = 412
	}
	if v := c.GetHeader("If-None-Match"); status == 200 && v != "" && matchesETag(v, etag, true) {
		status = 304
	}
	if v := c.GetHeader("If-Range"); v != "" && v != etag {
		rawRange = ""
	}
	offset, length, e := singleRange(rawRange, g.Size)
	if e != nil || len(c.Request.Header.Values("Range")) > 1 {
		if status == 200 {
			status = 416
		}
	} else if rawRange != "" && status == 200 {
		status = 206
	}
	e = g.Respond(ctx, func() error {
		c.Header("ETag", etag)
		c.Header("Cache-Control", "private, no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Accept-Ranges", "bytes")
		if status == 416 {
			c.Header("Content-Range", "bytes */"+strconv.FormatInt(g.Size, 10))
			c.Status(416)
			c.Writer.WriteHeaderNow()
			return nil
		}
		if status == 304 || status == 412 {
			c.Status(status)
			c.Writer.WriteHeaderNow()
			return nil
		}
		c.Header("Content-Type", g.MediaType)
		disposition := "attachment"
		if g.Variant == "preview" {
			disposition = "inline"
		}
		c.Header("Content-Disposition", disposition+`; filename="download"; filename*=UTF-8''`+strings.ReplaceAll(url.PathEscape(g.Name), "+", "%20"))
		c.Header("Content-Length", strconv.FormatInt(length, 10))
		if status == 206 {
			c.Header("Content-Range", "bytes "+strconv.FormatInt(offset, 10)+"-"+strconv.FormatInt(offset+length-1, 10)+"/"+strconv.FormatInt(g.Size, 10))
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
	e = g.Write(ctx, offset, length, func(data []byte) (int, error) {
		if e := controller.SetWriteDeadline(time.Now().Add(2 * time.Second)); e != nil {
			return 0, e
		}
		n, e := c.Writer.Write(data)
		if e == nil {
			e = controller.Flush()
		}
		return n, e
	})
	if e != nil {
		c.Abort()
	}
}

// ResponseController and range validators retain the same transport rules for professional results.
func ResponseController(c *gin.Context) *http.ResponseController {
	v, _ := c.Request.Context().Value(transportKey{}).(*http.ResponseController)
	return v
}
func SingleRange(raw string, size int64) (int64, int64, error) { return singleRange(raw, size) }
func MatchesETag(raw, etag string, weak bool) bool             { return matchesETag(raw, etag, weak) }
