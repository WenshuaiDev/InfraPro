package parsinghttp

import (
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	p "github.com/WenshuaiDev/InfraPro/packages/go/platform/parsing"
	h "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/devicecommon"
	ih "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/identity"
	"github.com/gin-gonic/gin"
	"strconv"
	"strings"
)

func Register(r *gin.Engine, identity *identity.Service, service *p.Service) {
	for _, policy := range access.Policies() {
		parts := strings.Split(string(policy.Operation), ":")
		if len(parts) != 4 || parts[0] != "Parsing" {
			continue
		}
		entry, kind, action := parts[1], parts[2], parts[3]
		r.Handle(policy.Method, policy.Path, func(ctx *gin.Context) {
			for _, param := range ctx.Params {
				if param.Key != "method_id" && param.Key != "version" && param.Key != "generation" && !access.ValidID(param.Value) {
					h.Reply(ctx, 0, nil, h.Invalid("路径对象标识无效。"))
					return
				}
			}
			scope := p.Scope{Entry: entry, Tenant: ctx.Param("tenant_id"), Project: ctx.Param("project_id")}
			actor := identity.AccessActor(ih.Principal(ctx))
			cc := ctx.Request.Context()
			var out any
			var err error
			status := 200
			product, version, device, config := ctx.Param("id"), ctx.Param("version_id"), ctx.Param("device_id"), ctx.Param("config_id")
			if ctx.Request.Method == "GET" {
				allowed := []string{}
				if action == "list" || action == "history" {
					allowed = []string{"q", "cursor", "limit", "state", "device_id"}
				}
				if kind == "runs" && (action == "result" || action == "response") {
					allowed = []string{"offset", "limit"}
				}
				if kind == "samples" && action == "download" {
					allowed = []string{"representation"}
				}
				filter, e := h.Filter(ctx, allowed...)
				if e != nil {
					h.Reply(ctx, 0, nil, e)
					return
				}
				switch kind + "." + action {
				case "device-configs.list":
					out, err = service.DeviceConfigs(cc, actor, scope, device, filter)
				case "ingress-protocols.list", "identification-methods.list", "ingress-compatibilities.list":
					out, err = service.Capabilities(cc, actor, scope, kind, filter)
				case "methods.list":
					out, err = service.Methods(cc, actor, scope, filter)
				case "catalog.status":
					out, err = service.CatalogStatus(cc, actor)
				case "catalog.detail":
					out, err = service.SyncDetail(cc, actor, ctx.Param("sync_id"))
				case "configs.list", "public-configs.list":
					out, err = service.Configs(cc, actor, scope, product, version, kind == "public-configs", filter)
				case "defaults.detail", "public-defaults.detail":
					out, err = service.Default(cc, actor, scope, product, version, kind == "public-defaults")
				case "device.detail":
					out, err = service.Device(cc, actor, scope, device)
				case "device.history":
					out, err = service.History(cc, actor, scope, device, filter)
				case "samples.list":
					out, err = service.Samples(cc, actor, scope, filter)
				case "samples.detail":
					out, err = service.Sample(cc, actor, scope, ctx.Param("sample_id"))
				case "samples.download":
					if ctx.Query("representation") != "" && ctx.Query("representation") != "original" {
						h.Reply(ctx, 0, nil, h.Invalid("样本表示无效。"))
						return
					}
					var raw []byte
					raw, err = service.SampleContent(cc, actor, scope, ctx.Param("sample_id"), ctx.Query("representation") == "original")
					if err == nil {
						ctx.Header("Content-Disposition", `attachment; filename="parsing-sample.bin"`)
						ctx.Data(200, "application/octet-stream", raw)
						return
					}
				case "runs.list":
					out, err = service.Runs(cc, actor, scope, filter)
				case "runs.detail":
					out, err = service.Run(cc, actor, scope, ctx.Param("run_id"))
				case "runs.execution":
					out, err = service.Execution(cc, actor, scope, ctx.Param("run_id"), ctx.Param("generation"))
				case "runs.result", "runs.response":
					offset := 0
					if ctx.Query("offset") != "" {
						offset, err = strconv.Atoi(ctx.Query("offset"))
						if err != nil {
							h.Reply(ctx, 0, nil, h.Invalid("结果分页无效。"))
							return
						}
					}
					limit := filter.Limit
					if limit == 0 {
						limit = 100
					}
					var raw []byte
					out, raw, err = service.Result(cc, actor, scope, ctx.Param("run_id"), ctx.Param("generation"), action == "response", offset, limit)
					if err == nil && action == "response" {
						ctx.Header("Content-Disposition", `attachment; filename="parsing-response.json"`)
						ctx.Data(200, "application/octet-stream", raw)
						return
					}
				default:
					err = h.Invalid("不支持的解析查询。")
				}
			} else {
				if _, err = h.Filter(ctx); err != nil {
					h.Reply(ctx, 0, nil, err)
					return
				}
				if kind == "samples" && action == "content" {
					if len(ctx.Request.Header.Values("X-Command-ID")) != 1 {
						h.Reply(ctx, 0, nil, h.Invalid("上传须提供 X-Command-ID 请求标识。"))
						return
					}
					out, err = service.UploadSample(cc, actor, scope, ctx.Param("sample_id"), p.Meta{RequestID: ctx.GetHeader("X-Command-ID")}, ctx.Request.Body)
					h.Reply(ctx, 200, out, err)
					return
				}
				raw, e := h.Body(ctx)
				if e != nil {
					h.Reply(ctx, 0, nil, e)
					return
				}
				decode := func(v any) error {
					if e := c.StrictJSON(raw, v); e != nil {
						return h.Invalid("请求 JSON、字段或类型无效。")
					}
					return nil
				}
				switch kind + "." + action {
				case "ingestion-catalog.sync":
					var input p.Meta
					if err = c.StrictJSON(raw, &input); err == nil {
						out, err = service.SyncIngestion(cc, actor, input)
					}
				case "ingress-protocols.state", "identification-methods.state", "ingress-compatibilities.state":
					var input p.StateInput
					if err = c.StrictJSON(raw, &input); err == nil {
						out, err = service.SetCapabilityState(cc, actor, kind, ctx.Param("capability_id"), input)
					}

				case "catalog.sync":
					var in p.Meta
					if err = decode(&in); err == nil {
						out, err = service.Sync(cc, actor, in)
					}
				case "methods.state":
					var in p.StateInput
					if err = decode(&in); err == nil {
						out, err = service.SetMethodState(cc, actor, ctx.Param("method_id"), ctx.Param("version"), in)
					}
				case "configs.create", "configs.revision", "device-configs.create", "device-configs.revision":
					var in p.ConfigInput
					if err = decode(&in); err == nil {
						out, err = service.SaveConfig(cc, actor, scope, product, version, device, config, in)
					}
				case "configs.state":
					var in p.StateInput
					if err = decode(&in); err == nil {
						out, err = service.SetConfigState(cc, actor, scope, product, version, config, in)
					}
				case "defaults.set":
					var in p.DefaultInput
					if err = decode(&in); err == nil {
						out, err = service.SetDefault(cc, actor, scope, product, version, in)
					}
				case "device.preview", "device.commands":
					var in p.Command
					if err = decode(&in); err == nil {
						if action == "preview" {
							out, err = service.Preview(cc, actor, scope, device, in)
						} else {
							out, err = service.Command(cc, actor, scope, device, in)
						}
					}
				case "batch.preview", "batch.commands":
					var in p.BatchCommand
					if err = decode(&in); err == nil {
						previews, result, e := service.Batch(cc, actor, scope, in, action == "preview")
						err = e
						if action == "preview" {
							out = previews
						} else {
							out = result
						}
					}
				case "samples.create":
					var in p.SampleInput
					if err = decode(&in); err == nil {
						out, err = service.CreateSample(cc, actor, scope, in)
					}
				case "samples.confirm":
					var in p.Meta
					if err = decode(&in); err == nil {
						out, err = service.ConfirmSample(cc, actor, scope, ctx.Param("sample_id"), in)
					}
				case "runs.create":
					var in p.RunInput
					if err = decode(&in); err == nil {
						out, err = service.CreateRun(cc, actor, scope, in)
						status = 202
					}
				case "runs.compare":
					var in p.CompareInput
					if err = decode(&in); err == nil {
						out, err = service.Compare(cc, actor, scope, in)
						status = 202
					}
				default:
					err = h.Invalid("不支持的解析操作。")
				}
			}
			h.Reply(ctx, status, out, err)
		})
	}
}
