package sourceshttp

import (
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/devices"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	s "github.com/WenshuaiDev/InfraPro/packages/go/platform/sources"
	h "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/devicecommon"
	ih "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/identity"
	"github.com/gin-gonic/gin"
	"strings"
)

func Register(r *gin.Engine, i *identity.Service, service *s.Service) {
	for _, p := range access.Policies() {
		parts := strings.Split(string(p.Operation), ":")
		if len(parts) != 4 || parts[0] != "Device" || (parts[2] != "source" && !(parts[2] == "output" && (parts[3] == "channel-candidates" || parts[3] == "source-history"))) {
			continue
		}
		entry, kind, action := parts[1], parts[2], parts[3]
		r.Handle(p.Method, p.Path, func(ctx *gin.Context) {
			if err := h.PathIDs(ctx); err != nil {
				h.Reply(ctx, 0, nil, err)
				return
			}
			scope := devices.TenantScope(ctx.Param("tenant_id"))
			if entry == "project" {
				scope = devices.ProjectScope(scope.Tenant, ctx.Param("project_id"))
			}
			a := i.AccessActor(ih.Principal(ctx))
			cc := ctx.Request.Context()
			target := s.Target{FacilityID: ctx.Param("facility_id"), PointID: ctx.Param("point_id"), ChannelID: ctx.Param("channel_id")}
			var out any
			var err error
			if ctx.Request.Method == "GET" {
				allowed := []string{}
				if action != "detail" {
					allowed = []string{"q", "cursor", "limit"}
				}
				if action == "candidates" {
					allowed = append(allowed, "device_id", "metric_version_id", "unit_version_id", "compatible_only")
				}
				if action == "channel-candidates" {
					allowed = append(allowed, "compatible_only")
				}
				var f s.Filter
				f, err = h.Filter(ctx, allowed...)
				if err != nil {
					h.Reply(ctx, 0, nil, err)
					return
				}
				switch kind + ":" + action {
				case "source:detail":
					out, err = service.Current(cc, a, scope, target)
				case "source:history":
					out, err = service.History(cc, a, scope, target, f)
				case "source:candidates":
					out, err = service.Candidates(cc, a, scope, target, f)
				case "output:channel-candidates":
					out, err = service.ChannelCandidates(cc, a, scope, ctx.Param("output_id"), f)
				case "output:source-history":
					out, err = service.OutputHistory(cc, a, scope, ctx.Param("output_id"), f)
				}
			} else {
				if _, err = h.Filter(ctx); err != nil {
					h.Reply(ctx, 0, nil, err)
					return
				}
				var raw []byte
				raw, err = h.Body(ctx)
				if err != nil {
					h.Reply(ctx, 0, nil, err)
					return
				}
				var command s.Command
				command, err = decode(raw, action)
				if err == nil {
					if action == "preview" {
						out, err = service.Preview(cc, a, scope, target, command)
					} else {
						out, err = service.Mutate(cc, a, scope, target, command, h.Meta(ctx))
					}
				}
			}
			h.Reply(ctx, 200, out, err)
		})
	}
}
func decode(raw []byte, action string) (s.Command, error) {
	preview := action == "preview"
	if preview {
		var v struct {
			Action string `json:"action"`
		}
		if json.Unmarshal(raw, &v) != nil {
			return s.Command{}, h.Invalid("请求格式无效。")
		}
		action = v.Action
	}
	switch action {
	case "bind":
		if preview {
			var in struct {
				Action string `json:"action"`
				s.BindInput
			}
			e := h.Decode(raw, &in)
			return s.Bind(in.BindInput), e
		}
		var in s.BindInput
		e := h.Decode(raw, &in)
		return s.Bind(in), e
	case "switch":
		if preview {
			var in struct {
				Action string `json:"action"`
				s.SwitchInput
			}
			e := h.Decode(raw, &in)
			return s.Switch(in.SwitchInput), e
		}
		var in s.SwitchInput
		e := h.Decode(raw, &in)
		return s.Switch(in), e
	case "unbind":
		if preview {
			var in struct {
				Action string `json:"action"`
				s.UnbindInput
			}
			e := h.Decode(raw, &in)
			return s.Unbind(in.UnbindInput), e
		}
		var in s.UnbindInput
		e := h.Decode(raw, &in)
		return s.Unbind(in), e
	}
	return s.Command{}, h.Invalid("请选择绑定、换源或解除来源。")
}
