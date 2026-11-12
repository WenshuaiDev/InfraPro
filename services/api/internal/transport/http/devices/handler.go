package deviceshttp

import (
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	d "github.com/WenshuaiDev/InfraPro/packages/go/platform/devices"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	h "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/devicecommon"
	ih "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/identity"
	"github.com/gin-gonic/gin"
	"slices"
	"strings"
)

func Register(r *gin.Engine, i *identity.Service, s *d.Service) {
	for _, p := range access.Policies() {
		parts := strings.Split(string(p.Operation), ":")
		if len(parts) != 4 || parts[0] != "Device" || !slices.Contains([]string{"device", "output", "installation", "tree", "access", "request"}, parts[2]) || slices.Contains([]string{"channel-candidates", "source-history"}, parts[3]) {
			continue
		}
		entry, kind, action := parts[1], parts[2], parts[3]
		r.Handle(p.Method, p.Path, func(ctx *gin.Context) {
			if err := h.PathIDs(ctx); err != nil {
				h.Reply(ctx, 0, nil, err)
				return
			}
			scope := c.PlatformScope()
			if entry == "tenant" {
				scope = d.TenantScope(ctx.Param("tenant_id"))
			}
			if entry == "project" {
				scope = d.ProjectScope(ctx.Param("tenant_id"), ctx.Param("project_id"))
			}
			a := i.AccessActor(ih.Principal(ctx))
			device, output, installation, project := ctx.Param("device_id"), ctx.Param("output_id"), ctx.Param("installation_id"), ctx.Param("project_id")
			cc := ctx.Request.Context()
			meta := h.Meta(ctx)
			var out any
			var err error
			if ctx.Request.Method == "GET" {
				allowed := []string{}
				if action != "detail" {
					allowed = []string{"q", "cursor", "limit"}
				}
				switch kind + ":" + action {
				case "tree:list":
					allowed = append(allowed, "level", "parent_id", "manufacturer_id", "product_id", "source", "status")
				case "device:list":
					allowed = append(allowed, "manufacturer_id", "product_id", "product_version_id", "status", "installation", "source")
				case "device:product-candidates":
					allowed = append(allowed, "product_id")
				case "output:list":
					allowed = append(allowed, "device_id", "manufacturer_id", "product_id", "status", "source")
				case "device:changes", "output:changes":
					allowed = append(allowed, "kind")
				case "output:generations":
					allowed = append(allowed, "slot_key")
				case "installation:list":
					allowed = append(allowed, "state")
				case "installation:targets":
					allowed = append(allowed, "kind", "state")
				case "access:detail", "access:candidates":
					allowed = []string{"q", "cursor", "limit", "kind", "device_id"}
				}
				var f d.Filter
				f, err = h.Filter(ctx, allowed...)
				if err != nil {
					h.Reply(ctx, 0, nil, err)
					return
				}
				switch kind + ":" + action {
				case "tree:list":
					out, err = s.Tree(cc, a, scope, f)
				case "device:list":
					out, err = s.List(cc, a, scope, f)
				case "device:detail":
					out, err = s.Detail(cc, a, scope, device)
				case "device:product-candidates":
					out, err = s.ProductCandidates(cc, a, scope, f)
				case "device:changes", "output:changes":
					out, err = s.Changes(cc, a, scope, device, output, f)
				case "output:list":
					out, err = s.Outputs(cc, a, scope, f)
				case "output:detail":
					out, err = s.Output(cc, a, scope, device, output)
				case "output:slots":
					out, err = s.Slots(cc, a, scope, device, f)
				case "output:generations":
					out, err = s.Generations(cc, a, scope, device, f)
				case "installation:list":
					out, err = s.Installations(cc, a, scope, device, f)
				case "installation:targets":
					if f.State != "" && f.State != "history" {
						err = h.Invalid("安装目标模式无效。")
					} else {
						out, err = s.InstallationTargets(cc, a, scope, device, f, f.State == "history")
					}
				case "access:detail":
					out, err = s.Grants(cc, a, scope.Tenant, project, f)
				case "access:candidates":
					out, err = s.GrantCandidates(cc, a, scope.Tenant, project, f)
				case "access:history":
					out, err = s.GrantChanges(cc, a, scope.Tenant, project, f)
				case "request:detail":
					out, err = s.Receipt(cc, a, scope, ctx.Param("request_id"))
				default:
					err = h.Invalid("不支持的查询。")
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
				if err = c.RejectNull(raw, "code", "name", "serial_number", "description", "status", "location_description", "installed_at", "moved_at", "output_slots", "add", "update", "end"); err != nil {
					h.Reply(ctx, 0, nil, err)
					return
				}
				if kind == "installation" || action == "preview" {
					if err = installationTimes(raw, action); err != nil {
						h.Reply(ctx, 0, nil, err)
						return
					}
				}
				switch kind + ":" + action {
				case "device:create":
					var in d.DeviceCreate
					if err = h.Decode(raw, &in); err == nil {
						out, err = s.Create(cc, a, scope, in, meta)
					}
				case "device:update":
					var in d.DeviceUpdate
					if err = h.Decode(raw, &in); err == nil {
						out, err = s.Mutate(cc, a, scope, device, d.DeviceMutation{Action: "update", DeviceUpdate: in}, meta)
					}
				case "device:state":
					var in d.DeviceState
					if err = h.Decode(raw, &in); err == nil {
						out, err = s.Mutate(cc, a, scope, device, d.DeviceMutation{Action: "state", DeviceUpdate: d.DeviceUpdate{RevisionInput: in.RevisionInput}, Status: in.Status}, meta)
					}
				case "device:delete":
					var in d.RevisionInput
					if err = h.Decode(raw, &in); err == nil {
						out, err = s.Mutate(cc, a, scope, device, d.DeviceMutation{Action: "delete", DeviceUpdate: d.DeviceUpdate{RevisionInput: in}}, meta)
					}
				case "device:preview":
					out, err = preview(ctx, s, a, scope, device, raw)
				case "output:create":
					var in d.OutputConfigure
					if err = h.Decode(raw, &in); err == nil {
						in.OutputFields.CapturePresence(raw)
						out, err = s.ConfigureOutput(cc, a, scope, device, in, meta)
					}
				case "output:update":
					var in d.OutputUpdate
					if err = h.Decode(raw, &in); err == nil {
						out, err = s.UpdateOutput(cc, a, scope, device, output, in, meta)
					}
				case "output:replace":
					var in d.OutputReplace
					if err = h.Decode(raw, &in); err == nil {
						in.OutputFields.CapturePresence(raw)
						out, err = s.ReplaceOutput(cc, a, scope, device, output, "replace", in, meta)
					}
				case "output:end":
					var in d.OutputEnd
					if err = h.Decode(raw, &in); err == nil {
						out, err = s.ReplaceOutput(cc, a, scope, device, output, "end", d.OutputReplace{RevisionInput: in.RevisionInput, OutputRevision: in.OutputRevision}, meta)
					}
				case "installation:create":
					var in d.InstallationCreate
					if err = h.Decode(raw, &in); err == nil {
						out, err = s.Install(cc, a, scope, device, d.InstallationCreateCommand(in), meta)
					}
				case "installation:end":
					var in d.InstallationEnd
					if err = h.Decode(raw, &in); err == nil {
						out, err = s.Install(cc, a, scope, device, d.InstallationEndCommand(installation, in), meta)
					}
				case "installation:relocate":
					var in d.InstallationRelocate
					if err = h.Decode(raw, &in); err == nil {
						out, err = s.Install(cc, a, scope, device, d.InstallationRelocateCommand(installation, in), meta)
					}
				case "installation:correct":
					var in d.InstallationCorrect
					if err = h.Decode(raw, &in); err == nil {
						out, err = s.Install(cc, a, scope, device, d.InstallationCorrectCommand(installation, in), meta)
					}
				case "access:preview", "access:change":
					var in d.GrantChangeSet
					if err = h.Decode(raw, &in); err == nil {
						if action == "preview" {
							out, err = s.PreviewGrants(cc, a, scope.Tenant, project, in)
						} else {
							out, err = s.ChangeGrants(cc, a, scope.Tenant, project, in, meta)
						}
					}
				default:
					err = h.Invalid("不支持的命令。")
				}
			}
			h.Reply(ctx, 200, out, err)
		})
	}
}
func preview(ctx *gin.Context, s *d.Service, a access.Actor, scope d.Scope, device string, raw []byte) (any, error) {
	var action struct {
		Action string `json:"action"`
	}
	if json.Unmarshal(raw, &action) != nil {
		return nil, h.Invalid("请求格式无效。")
	}
	cc := ctx.Request.Context()
	switch action.Action {
	case "update":
		var in struct {
			Action string `json:"action"`
			d.DeviceUpdate
		}
		if e := h.Decode(raw, &in); e != nil {
			return nil, e
		}
		return s.PreviewDevice(cc, a, scope, device, d.DeviceMutation{Action: in.Action, DeviceUpdate: in.DeviceUpdate})
	case "state":
		var in struct {
			Action string `json:"action"`
			d.DeviceState
		}
		if e := h.Decode(raw, &in); e != nil {
			return nil, e
		}
		return s.PreviewDevice(cc, a, scope, device, d.DeviceMutation{Action: in.Action, DeviceUpdate: d.DeviceUpdate{RevisionInput: in.RevisionInput}, Status: in.Status})
	case "delete":
		var in struct {
			Action string `json:"action"`
			d.RevisionInput
		}
		if e := h.Decode(raw, &in); e != nil {
			return nil, e
		}
		return s.PreviewDevice(cc, a, scope, device, d.DeviceMutation{Action: in.Action, DeviceUpdate: d.DeviceUpdate{RevisionInput: in.RevisionInput}})
	case "output_replace":
		var in struct {
			Action   string `json:"action"`
			OutputID string `json:"output_id"`
			d.OutputReplace
		}
		if e := h.Decode(raw, &in); e != nil {
			return nil, e
		}
		in.OutputFields.CapturePresence(raw)
		return s.PreviewOutput(cc, a, scope, device, in.OutputID, "replace", in.OutputReplace)
	case "output_end":
		var in struct {
			Action   string `json:"action"`
			OutputID string `json:"output_id"`
			d.OutputEnd
		}
		if e := h.Decode(raw, &in); e != nil {
			return nil, e
		}
		return s.PreviewOutput(cc, a, scope, device, in.OutputID, "end", d.OutputReplace{RevisionInput: in.RevisionInput, OutputRevision: in.OutputRevision})
	case "installation_create":
		var in struct {
			Action string `json:"action"`
			d.InstallationCreate
		}
		if e := h.Decode(raw, &in); e != nil {
			return nil, e
		}
		return s.PreviewInstallation(cc, a, scope, device, d.InstallationCreateCommand(in.InstallationCreate))
	case "installation_end":
		var in struct {
			Action         string `json:"action"`
			InstallationID string `json:"installation_id"`
			d.InstallationEnd
		}
		if e := h.Decode(raw, &in); e != nil {
			return nil, e
		}
		return s.PreviewInstallation(cc, a, scope, device, d.InstallationEndCommand(in.InstallationID, in.InstallationEnd))
	case "installation_relocate":
		var in struct {
			Action         string `json:"action"`
			InstallationID string `json:"installation_id"`
			d.InstallationRelocate
		}
		if e := h.Decode(raw, &in); e != nil {
			return nil, e
		}
		return s.PreviewInstallation(cc, a, scope, device, d.InstallationRelocateCommand(in.InstallationID, in.InstallationRelocate))
	case "installation_correct":
		var in struct {
			Action         string `json:"action"`
			InstallationID string `json:"installation_id"`
			d.InstallationCorrect
		}
		if e := h.Decode(raw, &in); e != nil {
			return nil, e
		}
		return s.PreviewInstallation(cc, a, scope, device, d.InstallationCorrectCommand(in.InstallationID, in.InstallationCorrect))
	}
	return nil, h.Invalid("请选择有效的预览操作。")
}
