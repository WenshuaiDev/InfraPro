package derivedmonitoringhttp

import (
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	"github.com/gin-gonic/gin"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func analysis(c *gin.Context, s *dm.Service, a access.Actor, scope dm.Scope, action string, f dm.MetricFilter, meta access.Meta) {
	ctx, id := c.Request.Context(), c.Param("id")
	generation := strings.HasPrefix(action, "evidence.")
	if generation {
		action = strings.Replace(action, "evidence.", "analysis.", 1)
	}
	switch action {
	case "analysis.artifacts.exports":
		var in dm.AnalysisExportInput
		if e := body(c, &in); e != nil {
			reply(c, 0, nil, e)
			return
		}
		v, e := s.CreateAnalysisExport(ctx, a, scope, id, in, meta)
		reply(c, 202, v, e)
	case "analysis.exports.list":
		list := s.AnalysisExports
		if generation {
			list = s.GenerationExports
		}
		v, e := list(ctx, a, scope, f)
		reply(c, 200, v, e)
	case "analysis.exports.detail":
		load := s.AnalysisExport
		if generation {
			load = s.GenerationExport
		}
		v, e := load(ctx, a, scope, id)
		reply(c, 200, v, e)
	case "analysis.exports.files":
		files := s.AnalysisExportFiles
		if generation {
			files = s.GenerationExportFiles
		}
		v, e := files(ctx, a, scope, id)
		reply(c, 200, v, e)
	case "analysis.exports.cancel", "analysis.exports.retry":
		var in dm.WriteMeta
		if e := body(c, &in); e != nil {
			reply(c, 0, nil, e)
			return
		}
		control := s.ControlAnalysisExport
		if generation {
			control = s.ControlGenerationExport
		}
		v, e := control(ctx, a, scope, id, strings.TrimPrefix(action, "analysis.exports."), in, meta)
		reply(c, 200, v, e)
	case "analysis.exports.download":
		download := s.AnalysisExportDownload
		if generation {
			download = s.GenerationExportDownload
		}
		file, e := download(ctx, a, scope, id, c.Param("part"))
		if e != nil {
			reply(c, 0, nil, e)
			return
		}
		_ = http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(30 * time.Minute))
		c.Header("Content-Type", file.ContentType)
		c.Header("Content-Length", strconv.FormatInt(file.Size, 10))
		c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": file.Name}))
		c.Header("Cache-Control", "private, no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Content-SHA256", file.SHA256)
		if e = file.Write(ctx, c.Writer); e != nil {
			if !c.Writer.Written() {
				c.Header("Content-Length", "")
				reply(c, 0, nil, e)
			} else {
				c.Abort()
			}
		}

	case "analysis.comparisons":
		var in dm.AnalysisCompareInput
		if e := body(c, &in); e != nil {
			reply(c, 0, nil, e)
			return
		}
		v, e := s.CompareAnalysis(ctx, a, scope, in, meta)
		reply(c, 202, v, e)
	case "analysis.comparisons.query":
		var in dm.AnalysisComparisonQuery
		if e := body(c, &in); e != nil {
			reply(c, 0, nil, e)
			return
		}
		v, e := s.AnalysisComparisonRows(ctx, a, scope, id, in)
		reply(c, 200, v, e)
	case "analysis.configs.list":
		v, e := s.AnalysisConfigs(ctx, a, scope, f)
		reply(c, 200, v, e)
	case "analysis.configs.detail":
		v, e := s.AnalysisConfig(ctx, a, scope, id)
		reply(c, 200, v, e)
	case "analysis.configs.create":
		var in dm.CreateAnalysisInput
		if e := body(c, &in); e != nil {
			reply(c, 0, nil, e)
			return
		}
		v, e := s.CreateAnalysis(ctx, a, scope, in, meta)
		reply(c, 201, v, e)
	case "analysis.configs.save":
		var in dm.SaveAnalysisInput
		if e := body(c, &in); e != nil {
			reply(c, 0, nil, e)
			return
		}
		v, e := s.SaveAnalysis(ctx, a, scope, id, in, meta)
		reply(c, 200, v, e)
	case "analysis.configs.copy":
		var in dm.CopyAnalysisInput
		if e := body(c, &in); e != nil {
			reply(c, 0, nil, e)
			return
		}
		v, e := s.CopyAnalysis(ctx, a, scope, id, in, meta)
		reply(c, 201, v, e)
	case "analysis.configs.validate":
		var in dm.ValidateInput
		if e := body(c, &in); e != nil {
			reply(c, 0, nil, e)
			return
		}
		v, e := s.ValidateAnalysis(ctx, a, scope, id, in)
		reply(c, 200, v, e)
	case "analysis.configs.publish":
		var in dm.PublishAnalysisInput
		if e := body(c, &in); e != nil {
			reply(c, 0, nil, e)
			return
		}
		v, e := s.PublishAnalysis(ctx, a, scope, id, in, meta)
		reply(c, 201, v, e)
	case "analysis.configs.versions":
		v, e := s.AnalysisVersions(ctx, a, scope, id, f.Cursor)
		reply(c, 200, v, e)
	case "analysis.configs.archive":
		var in dm.WriteMeta
		if e := body(c, &in); e != nil {
			reply(c, 0, nil, e)
			return
		}
		v, e := s.ArchiveAnalysis(ctx, a, scope, id, in, meta)
		reply(c, 200, v, e)
	case "analysis.trials", "analysis.runs":
		var in dm.AnalysisRunInput
		if e := body(c, &in); e != nil {
			reply(c, 0, nil, e)
			return
		}
		v, e := s.StartAnalysis(ctx, a, scope, in, action == "analysis.trials", meta)
		reply(c, 202, v, e)
	case "analysis.artifacts.list":
		v, e := s.AnalysisArtifacts(ctx, a, scope, f)
		reply(c, 200, v, e)
	case "analysis.artifacts.detail":
		v, e := s.AnalysisArtifact(ctx, a, scope, id, c.Query("version_id"))
		reply(c, 200, v, e)
	case "analysis.artifacts.versions":
		v, e := s.AnalysisArtifactVersions(ctx, a, scope, id, f.Cursor)
		reply(c, 200, v, e)
	case "analysis.artifacts.query":
		var in dm.AnalysisArtifactQuery
		if e := body(c, &in); e != nil {
			reply(c, 0, nil, e)
			return
		}
		v, e := s.AnalysisArtifactRows(ctx, a, scope, id, in)
		reply(c, 200, v, e)
	case "analysis.artifacts.lineage":
		var in dm.AnalysisArtifactQuery
		if e := body(c, &in); e != nil {
			reply(c, 0, nil, e)
			return
		}
		v, e := s.AnalysisArtifactLineage(ctx, a, scope, id, in)
		reply(c, 200, v, e)
	case "analysis.artifacts.archive":
		var in dm.WriteMeta
		if e := body(c, &in); e != nil {
			reply(c, 0, nil, e)
			return
		}
		v, e := s.ArchiveAnalysisArtifact(ctx, a, scope, id, in, meta)
		reply(c, 200, v, e)
	default:
		reply(c, 0, nil, &dm.Error{Code: "dependency_unavailable", Message: "分析操作尚不可用。", Status: 503, Issues: []dm.Issue{}})
	}
}
