// Package ingestionhttp exposes business authorization only; cloud listeners
// are deliberately absent from the console API.
package ingestionhttp

import (
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	p "github.com/WenshuaiDev/InfraPro/packages/go/platform/ingestion"
	h "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/devicecommon"
	ih "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/identity"
	"github.com/gin-gonic/gin"
	"strconv"
	"strings"
	"time"
)

func Register(r *gin.Engine, identity *identity.Service, service *p.Service) {
	for _, policy := range access.Policies() {
		parts := strings.Split(string(policy.Operation), ":")
		if len(parts) != 4 || parts[0] != "Ingestion" {
			continue
		}
		entry, kind, action := parts[1], parts[2], parts[3]
		r.Handle(policy.Method, policy.Path, func(ctx *gin.Context) {
			for _, param := range ctx.Params {
				valid := access.ValidID(param.Value)
				if param.Key == "generation" {
					n, e := strconv.ParseInt(param.Value, 10, 64)
					valid = e == nil && n > 0
				}
				if !valid {
					h.Reply(ctx, 0, nil, h.Invalid("对象标识或执行代次无效。"))
					return
				}
			}

			scope := p.Scope{Entry: entry, Tenant: ctx.Param("tenant_id"), Project: ctx.Param("project_id")}
			actor := identity.AccessActor(ih.Principal(ctx))
			cc := ctx.Request.Context()
			var out any
			var err error
			if ctx.Request.Method == "GET" {
				allowed := []string{}
				if action == "list" {
					allowed = []string{"q", "cursor", "limit", "state", "source_id", "source_project_id", "terminal_id", "device_id", "receipt_id", "method_version", "error_code", "from", "until"}
				}
				filter, e := h.Filter(ctx, allowed...)
				if e != nil {
					h.Reply(ctx, 0, nil, e)
					return
				}
				f := p.Filter{Filter: filter, SourceID: ctx.Query("source_id"), ProjectID: ctx.Query("source_project_id"), TerminalID: ctx.Query("terminal_id"), DeviceID: ctx.Query("device_id"), ReceiptID: ctx.Query("receipt_id"), MethodVersion: ctx.Query("method_version"), ErrorCode: ctx.Query("error_code")}
				for _, v := range []string{f.SourceID, f.ProjectID, f.TerminalID, f.DeviceID, f.ReceiptID} {
					if v != "" && !access.ValidID(v) {
						h.Reply(ctx, 0, nil, h.Invalid("筛选对象标识无效。"))
						return
					}
				}
				for key, target := range map[string]**time.Time{"from": &f.From, "until": &f.Until} {
					if ctx.Query(key) != "" {
						v, e := time.Parse(time.RFC3339Nano, ctx.Query(key))
						if e != nil {
							h.Reply(ctx, 0, nil, h.Invalid("时间须为 RFC3339。"))
							return
						}
						*target = &v
					}
				}
				if f.From != nil && f.Until != nil && !f.From.Before(*f.Until) {
					h.Reply(ctx, 0, nil, h.Invalid("结束时间须晚于开始时间。"))
					return
				}
				switch kind + "." + action {
				case "integrity-events.list":
					out, err = service.IntegrityEvents(cc, actor)
				case "source-options.list":
					out, err = service.SourceOptions(cc, actor)
				case "relays.list":
					out, err = service.Relays(cc, actor)
				case "sources.list":
					out, err = service.Sources(cc, actor, scope, f)
				case "sources.detail":
					out, err = service.Source(cc, actor, scope, ctx.Param("source_id"))
				case "terminals.list":
					out, err = service.Terminals(cc, actor, scope, f)
				case "terminals.history":
					out, err = service.TerminalHistory(cc, actor, scope, ctx.Param("terminal_id"))
				case "terminals.detail":
					out, err = service.Terminal(cc, actor, scope, ctx.Param("terminal_id"))
				case "associations.list":
					out, err = service.Associations(cc, actor, scope, ctx.Param("terminal_id"))
				case "execution-grants.list":
					out, err = service.Grants(cc, actor, scope, f)
				case "messages.list":
					out, err = service.Messages(cc, actor, scope, f)
				case "messages.detail":
					out, err = service.Message(cc, actor, scope, ctx.Param("message_id"))
				case "messages.decisions":
					out, err = service.Decisions(cc, actor, scope, ctx.Param("message_id"))
				case "messages.results":
					out, _, err = service.Results(cc, actor, scope, ctx.Param("message_id"), "")
				case "messages.content", "messages.response":
					var raw []byte
					result := action == "response"
					if result {
						_, raw, err = service.Results(cc, actor, scope, ctx.Param("message_id"), ctx.Param("result_id"))
					} else {
						raw, err = service.Content(cc, actor, scope, ctx.Param("message_id"))
					}
					if err == nil {
						start, end, ok := rangeBounds(ctx.GetHeader("Range"), len(raw))
						ctx.Header("Cache-Control", "private, no-store")
						ctx.Header("Accept-Ranges", "bytes")
						if !ok {
							ctx.Header("Content-Range", "bytes */"+strconv.Itoa(len(raw)))
							ctx.Status(416)
							return
						}
						status := 200
						if ctx.GetHeader("Range") != "" {
							status = 206
							ctx.Header("Content-Range", "bytes "+strconv.Itoa(start)+"-"+strconv.Itoa(end-1)+"/"+strconv.Itoa(len(raw)))
						}
						raw = raw[start:end]
						// Recheck on every bounded chunk, including after object retrieval.
						for offset := 0; offset < len(raw); offset += 16384 {
							if err = service.CheckContent(cc, actor, scope, ctx.Param("message_id"), result); err != nil {
								if offset == 0 {
									h.Reply(ctx, 0, nil, err)
								}
								return
							}
							if offset == 0 {
								ctx.Header("Cache-Control", "private, no-store")
								ctx.Status(status)
								ctx.Header("Content-Type", "application/octet-stream")
								ctx.Header("Content-Length", strconv.Itoa(len(raw)))
								ctx.Header("Content-Disposition", `attachment; filename="ingestion-unit.bin"`)
							}
							end := min(offset+16384, len(raw))
							if _, e = ctx.Writer.Write(raw[offset:end]); e != nil {
								return
							}
							ctx.Writer.Flush()
						}
						return
					}
				case "unmatched.list", "quarantine.list":
					out, err = service.Unmatched(cc, actor, scope, f)
				case "recovery.detail":
					out, err = service.RecoveryGeneration(cc, actor, scope, ctx.Param("recovery_id"), ctx.Param("generation"))
				case "recovery.list":
					out, err = service.Recoveries(cc, actor, scope)
				case "operations.list":
					out, err = service.Operations(cc, actor, scope, f)
				case "device.detail":
					out, err = service.DeviceSummary(cc, actor, scope, ctx.Param("device_id"))
				default:
					err = h.Invalid("不支持的接入查询。")
				}
			} else {
				if _, err = h.Filter(ctx); err != nil {
					h.Reply(ctx, 0, nil, err)
					return
				}
				raw, e := h.Body(ctx)
				if e != nil {
					h.Reply(ctx, 0, nil, e)
					return
				}
				switch kind + "." + action {
				case "relays.create":
					var in p.RelayInput
					if err = h.Decode(raw, &in); err == nil {
						out, err = service.CreateRelay(cc, actor, in)
					}
				case "sources.create", "sources.revision":
					var in p.SourceInput
					if err = h.Decode(raw, &in); err == nil {
						out, err = service.SaveSource(cc, actor, ctx.Param("source_id"), in)
					}
				case "sources.commands":
					var in p.Command
					if err = h.Decode(raw, &in); err == nil {
						out, err = service.SourceCommand(cc, actor, ctx.Param("source_id"), in)
					}
				case "sources.confirm":
					var in p.Command
					if err = h.Decode(raw, &in); err == nil {
						out, err = service.ConfirmSource(cc, actor, scope, ctx.Param("source_id"), in)
					}
				case "sources.planning":
					var in p.Command
					if err = h.Decode(raw, &in); err == nil {
						out, err = service.PlanningState(cc, actor, scope, ctx.Param("source_id"), in)
					}
				case "terminals.revision":
					var in p.TerminalInput
					if err = h.Decode(raw, &in); err == nil {
						out, err = service.ReviseTerminal(cc, actor, scope, ctx.Param("terminal_id"), in)
					}
				case "terminals.create":
					var in p.TerminalInput
					if err = h.Decode(raw, &in); err == nil {
						out, err = service.CreateTerminal(cc, actor, scope, in)
					}
				case "associations.create", "associations.preview":
					var in p.AssociationInput
					if err = h.Decode(raw, &in); err == nil {
						out, err = service.ChangeAssociation(cc, actor, scope, ctx.Param("terminal_id"), in, action == "preview")
					}
				case "associations.state":
					var in p.Command
					if err = h.Decode(raw, &in); err == nil {
						out, err = service.AssociationState(cc, actor, scope, ctx.Param("association_id"), in)
					}
				case "changes.cancel":
					var in p.Meta
					if err = h.Decode(raw, &in); err == nil {
						out, err = service.CancelChange(cc, actor, scope, ctx.Param("change_id"), in)
					}
				case "execution-grants.save":
					var in p.GrantInput
					if err = h.Decode(raw, &in); err == nil {
						out, err = service.SaveGrant(cc, actor, scope, in)
					}
				case "unmatched.close":
					var in p.Meta
					if err = h.Decode(raw, &in); err == nil {
						out, err = service.CloseUnmatched(cc, actor, scope, ctx.Param("receipt_id"), in)
					}
				case "recovery.create", "recovery.preview":
					var in p.RecoveryInput
					if err = h.Decode(raw, &in); err == nil {
						out, err = service.Recover(cc, actor, scope, in, action == "preview")
					}
				default:
					err = h.Invalid("不支持的接入操作。")
				}
			}
			h.Reply(ctx, 200, out, err)
		})
	}
}

// Ranges are interpreted in the verified unit's byte space. Never clamp an
// excessive requested end or return unrelated bytes from its parent object.
func rangeBounds(header string, size int) (int, int, bool) {
	if header == "" {
		return 0, size, true
	}
	if !strings.HasPrefix(header, "bytes=") || strings.Contains(header, ",") {
		return 0, 0, false
	}
	parts := strings.Split(strings.TrimPrefix(header, "bytes="), "-")
	if len(parts) != 2 {
		return 0, 0, false
	}
	if parts[0] == "" {
		n, e := strconv.Atoi(parts[1])
		if e != nil || n <= 0 || n > size {
			return 0, 0, false
		}
		return size - n, size, true
	}
	start, e := strconv.Atoi(parts[0])
	if e != nil || start < 0 || start >= size {
		return 0, 0, false
	}
	end := size - 1
	if parts[1] != "" {
		end, e = strconv.Atoi(parts[1])
		if e != nil || end < start || end >= size {
			return 0, 0, false
		}
	}
	return start, end + 1, true
}
