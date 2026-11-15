package derivedmonitoringhttp

import (
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	identityhttp "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/identity"
	"github.com/gin-gonic/gin"
)

func invalid(message string) *dm.Error {
	return &dm.Error{Code: "malformed_request", Message: message, Status: 400, Issues: []dm.Issue{}}
}
func reply(c *gin.Context, status int, value any, err error) {
	if err != nil {
		access.ReportOperationFailure(c.Request.Context(), err)
		var e *dm.Error
		if errors.As(err, &e) {
			c.AbortWithStatusJSON(e.Status, e)
			return
		}
		var raw *md.Error
		if errors.As(err, &raw) {
			c.AbortWithStatusJSON(raw.Status, raw)
			return
		}
		identityhttp.RespondError(c, err)
		return
	}
	c.JSON(status, value)
}
func body(c *gin.Context, out any) error {
	b, e := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 256*1024))
	if e != nil {
		var large *http.MaxBytesError
		if errors.As(e, &large) {
			return &dm.Error{Code: "input_too_large", Message: "单次请求最多 256 KiB。", Status: 413, Issues: []dm.Issue{}}
		}
		return invalid("请求读取失败。")
	}
	return md.DecodeRequest(b, out)
}
func filter(c *gin.Context, allowed ...string) (dm.MetricFilter, error) {
	out := dm.MetricFilter{Query: c.Query("q"), State: c.Query("state"), Cursor: c.Query("cursor")}
	for key, values := range c.Request.URL.Query() {
		if len(values) != 1 || !slices.Contains(allowed, key) {
			return out, invalid("不支持或重复的查询参数。")
		}
	}
	if text := c.Query("limit"); text != "" {
		v, e := strconv.Atoi(text)
		if e != nil || v < 1 || v > 100 {
			return out, invalid("分页大小须为 1–100。")
		}
		out.Limit = v
	}
	return out, nil
}
func Register(r *gin.Engine, identityService *identity.Service, s *dm.Service) {
	for _, policy := range access.Policies() {
		parts := strings.SplitN(string(policy.Operation), ":", 3)
		if len(parts) != 3 || parts[0] != "Derived" {
			continue
		}
		entry, action := parts[1], parts[2]
		r.Handle(policy.Method, policy.Path, func(c *gin.Context) {
			for _, p := range c.Params {
				valid := p.Key == "template" || access.ValidID(p.Value)
				if p.Key == "part" {
					n, err := strconv.ParseUint(p.Value, 10, 32)
					valid = p.Value == "manifest" || err == nil && n > 0 && strconv.FormatUint(n, 10) == p.Value
				}
				if !valid {
					reply(c, 0, nil, &dm.Error{Code: "not_found", Message: "对象不存在或不可访问。", Status: 404, Issues: []dm.Issue{}})
					return
				}
			}
			allowed := []string{}
			switch action {
			case "channels.list":
				allowed = []string{"q", "control_state", "facility_id", "owner_kind", "owner_id", "cursor", "limit"}
			case "runs.list":
				allowed = []string{"channel_id", "state", "cursor", "limit"}
			case "metrics.list", "analysis.configs.list", "analysis.artifacts.list":
				allowed = []string{"q", "state", "cursor", "limit"}
			case "channels.generations":
				allowed = []string{"from_ns", "to_ns", "limit", "cursor"}
			case "channels.assignments":
				allowed = []string{"revision", "cursor"}
			case "evidence.exports.list", "analysis.exports.list", "metrics.versions", "channels.configs.list", "analysis.configs.versions", "analysis.artifacts.versions":
				allowed = []string{"cursor", "limit"}
			case "analysis.artifacts.detail":
				allowed = []string{"version_id"}
			}
			f, e := filter(c, allowed...)
			if e != nil {
				reply(c, 0, nil, e)
				return
			}
			ctx := c.Request.Context()
			a := identityService.AccessActor(identityhttp.Principal(c))
			scope := dm.TenantScope(c.Param("tenant_id"))
			if entry == "project" {
				scope = dm.ProjectScope(scope.Tenant, c.Param("project_id"))
			}
			id := c.Param("id")
			meta := access.Meta{ActorType: "account", IP: c.ClientIP(), UserAgent: c.Request.UserAgent(), RequestID: c.Writer.Header().Get("X-Request-ID")}
			if strings.HasPrefix(action, "analysis.") || strings.HasPrefix(action, "evidence.") {
				analysis(c, s, a, scope, action, f, meta)
				return
			}
			switch action {
			case "generations.exports":
				var in dm.GenerationExportInput
				if e := body(c, &in); e != nil {
					reply(c, 0, nil, e)
					return
				}
				v, e := s.CreateGenerationExport(ctx, a, scope, id, in, meta)
				reply(c, 202, v, e)
			case "generations.replay":
				var in dm.ReplayInput
				if e := body(c, &in); e != nil {
					reply(c, 0, nil, e)
					return
				}
				v, e := s.ReplayGeneration(ctx, a, scope, id, in, meta)
				reply(c, 202, v, e)
			case "channels.generations":
				var to *string
				if text := c.Query("to_ns"); text != "" {
					to = &text
				}
				v, e := s.ChannelGenerations(ctx, a, scope, id, dm.GenerationFilter{Range: dm.NanosecondRange{From: c.Query("from_ns"), To: to}, Cursor: f.Cursor, Limit: f.Limit})
				reply(c, 200, v, e)
			case "generations.query", "generations.lineage":
				var in dm.GenerationQuery
				if e := body(c, &in); e != nil {
					reply(c, 0, nil, e)
					return
				}
				if action == "generations.query" {
					v, e := s.GenerationDetails(ctx, a, scope, id, in)
					reply(c, 200, v, e)
				} else {
					v, e := s.GenerationLineage(ctx, a, scope, id, in)
					reply(c, 200, v, e)
				}
			case "channels.backfill-preview":
				var in dm.BackfillPreviewInput
				if e := body(c, &in); e != nil {
					reply(c, 0, nil, e)
					return
				}
				v, e := s.PreviewBackfill(ctx, a, scope, id, in)
				reply(c, 200, v, e)
			case "channels.backfill":
				var in dm.ControlInput
				if e := body(c, &in); e != nil {
					reply(c, 0, nil, e)
					return
				}
				v, e := s.BackfillChannel(ctx, a, scope, id, in, meta)
				reply(c, 202, v, e)
			case "channels.configs.create":
				var in dm.SaveConfigInput
				if e := body(c, &in); e != nil {
					reply(c, 0, nil, e)
					return
				}
				v, e := s.SaveChannelConfig(ctx, a, scope, id, in, meta)
				reply(c, 201, v, e)
			case "channels.assignments":
				v, e := s.ChannelAssignments(ctx, a, scope, id, c.Query("revision"), c.Query("cursor"))
				reply(c, 200, v, e)
			case "channels.config-preview":
				var in dm.ConfigPreviewInput
				if e := body(c, &in); e != nil {
					reply(c, 0, nil, e)
					return
				}
				v, e := s.PreviewConfig(ctx, a, scope, id, in)
				reply(c, 200, v, e)
			case "channels.apply-config":
				var in dm.ControlInput
				if e := body(c, &in); e != nil {
					reply(c, 0, nil, e)
					return
				}
				v, e := s.ApplyConfig(ctx, a, scope, id, in, meta)
				reply(c, 200, v, e)
			case "channels.control-preview":
				var in dm.ControlPreviewInput
				if e := body(c, &in); e != nil {
					reply(c, 0, nil, e)
					return
				}
				v, e := s.PreviewControl(ctx, a, scope, id, in)
				reply(c, 200, v, e)
			case "channels.control":
				var in dm.ControlInput
				if e := body(c, &in); e != nil {
					reply(c, 0, nil, e)
					return
				}
				v, e := s.ControlChannel(ctx, a, scope, id, in, meta)
				reply(c, 200, v, e)
			case "channels.trials":
				var in dm.TrialInput
				if e := body(c, &in); e != nil {
					reply(c, 0, nil, e)
					return
				}
				v, e := s.ChannelTrial(ctx, a, scope, id, in, meta)
				status := 202
				if v.Job.State == "succeeded" {
					status = 200
				}
				reply(c, status, v, e)
			case "runs.list":
				v, e := s.Runs(ctx, a, scope, dm.RunFilter{ChannelID: c.Query("channel_id"), State: f.State, Cursor: f.Cursor, Limit: f.Limit})
				reply(c, 200, v, e)
			case "runs.detail":
				v, e := s.Run(ctx, a, scope, id)
				reply(c, 200, v, e)
			case "runs.cancel", "runs.retry":
				var in dm.WriteMeta
				if e := body(c, &in); e != nil {
					reply(c, 0, nil, e)
					return
				}
				v, e := s.ControlRun(ctx, a, scope, id, strings.TrimPrefix(action, "runs."), in, meta)
				reply(c, 200, v, e)
			case "channels.list":
				v, e := s.Channels(ctx, a, scope, dm.ChannelFilter{Query: f.Query, Cursor: f.Cursor, Limit: f.Limit, ControlState: c.Query("control_state"), FacilityID: c.Query("facility_id"), OwnerKind: c.Query("owner_kind"), OwnerID: c.Query("owner_id")})
				reply(c, 200, v, e)
			case "channels.detail":
				v, e := s.Channel(ctx, a, scope, id)
				reply(c, 200, v, e)
			case "channels.configs.list":
				v, e := s.ChannelConfigs(ctx, a, scope, id, f)
				reply(c, 200, v, e)
			case "channels.dependencies":
				v, e := s.ChannelDependencies(ctx, a, scope, id)
				reply(c, 200, v, e)
			case "channels.create":
				var in dm.CreateChannelInput
				if e := body(c, &in); e != nil {
					reply(c, 0, nil, e)
					return
				}
				v, e := s.CreateChannel(ctx, a, scope, in, meta)
				reply(c, 201, v, e)
			case "templates.list":
				v, e := s.Templates(ctx, a, scope)
				reply(c, 200, v, e)
			case "templates.expand":
				var in dm.ExpandTemplateInput
				if e := body(c, &in); e != nil {
					reply(c, 0, nil, e)
					return
				}
				v, e := s.ExpandTemplate(ctx, a, scope, c.Param("template"), in)
				reply(c, 200, v, e)
			case "metrics.list":
				v, e := s.Metrics(ctx, a, scope, f)
				reply(c, 200, v, e)
			case "metrics.detail":
				v, e := s.Metric(ctx, a, scope, id)
				reply(c, 200, v, e)
			case "metrics.versions":
				v, e := s.MetricVersions(ctx, a, scope, id, f)
				reply(c, 200, v, e)
			case "versions.detail":
				v, e := s.MetricVersion(ctx, a, scope, id)
				reply(c, 200, v, e)
			case "metrics.create":
				var in dm.CreateMetricInput
				if e := body(c, &in); e != nil {
					reply(c, 0, nil, e)
					return
				}
				v, e := s.CreateMetric(ctx, a, scope, in, meta)
				reply(c, 201, v, e)
			case "metrics.save":
				var in dm.SaveMetricInput
				if e := body(c, &in); e != nil {
					reply(c, 0, nil, e)
					return
				}
				v, e := s.SaveMetricDraft(ctx, a, scope, id, in, meta)
				reply(c, 200, v, e)
			case "metrics.copy":
				var in dm.CopyMetricInput
				if e := body(c, &in); e != nil {
					reply(c, 0, nil, e)
					return
				}
				v, e := s.CopyMetric(ctx, a, scope, id, in, meta)
				reply(c, 201, v, e)
			case "metrics.validate":
				var in dm.ValidateInput
				if e := body(c, &in); e != nil {
					reply(c, 0, nil, e)
					return
				}
				v, e := s.ValidateMetric(ctx, a, scope, id, in)
				reply(c, 200, v, e)
			case "metrics.publish":
				var in dm.PublishMetricInput
				if e := body(c, &in); e != nil {
					reply(c, 0, nil, e)
					return
				}
				v, e := s.PublishMetric(ctx, a, scope, id, in, meta)
				reply(c, 201, v, e)
			case "versions.state":
				var in dm.VersionStateInput
				if e := body(c, &in); e != nil {
					reply(c, 0, nil, e)
					return
				}
				v, e := s.SetMetricVersionState(ctx, a, scope, id, in, meta)
				reply(c, 200, v, e)
			case "metrics.delete":
				var in dm.WriteMeta
				if e := body(c, &in); e != nil {
					reply(c, 0, nil, e)
					return
				}
				v, e := s.DeleteMetricDraft(ctx, a, scope, id, in, meta)
				reply(c, 200, v, e)
			default:
				reply(c, 0, nil, invalid("不支持的派生命令。"))
			}
		})
	}
}
