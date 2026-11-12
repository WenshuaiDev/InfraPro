package devicecataloghttp

import (
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/identity"
	h "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/devicecommon"
	identityhttp "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/identity"
	"github.com/gin-gonic/gin"
	"slices"
	"strings"
)

func Register(r *gin.Engine, i *identity.Service, s *c.Service) {
	for _, policy := range access.Policies() {
		parts := strings.Split(string(policy.Operation), ":")
		if len(parts) != 4 || parts[0] != "Device" || !slices.Contains([]string{"manufacturer", "product", "catalog", "catalog-manufacturer", "catalog-product"}, parts[2]) {
			continue
		}
		entry, kind, action := parts[1], parts[2], parts[3]
		r.Handle(policy.Method, policy.Path, func(ctx *gin.Context) {
			if err := h.PathIDs(ctx); err != nil {
				h.Reply(ctx, 0, nil, err)
				return
			}
			scope := c.PlatformScope()
			if entry == "tenant" {
				scope = c.TenantScope(ctx.Param("tenant_id"))
			}
			actor := i.AccessActor(identityhttp.Principal(ctx))
			id := ctx.Param("id")
			version := ctx.Param("version_id")
			meta := h.Meta(ctx)
			var result any
			var err error
			if ctx.Request.Method == "GET" {
				allowed := []string{}
				if slices.Contains([]string{"list", "versions", "changes"}, action) || kind == "product" && action == "detail" {
					allowed = []string{"q", "cursor", "limit"}
				}
				if action == "list" {
					allowed = append(allowed, "manufacturer_id", "status")
				}
				if action == "changes" {
					allowed = append(allowed, "manufacturer_id", "product_id")
				}
				var f c.Filter
				f, err = h.Filter(ctx, allowed...)
				if err != nil {
					h.Reply(ctx, 0, nil, err)
					return
				}
				switch kind + ":" + action {
				case "manufacturer:list", "catalog-manufacturer:list":
					result, err = s.Manufacturers(ctx.Request.Context(), actor, scope, f, kind == "catalog-manufacturer")
				case "manufacturer:detail":
					result, err = s.Manufacturer(ctx.Request.Context(), actor, scope, id)
				case "product:list", "catalog-product:list":
					result, err = s.Products(ctx.Request.Context(), actor, scope, f, kind == "catalog-product")
				case "product:detail":
					result, err = s.Product(ctx.Request.Context(), actor, scope, id, f)
				case "product:version", "catalog-product:version":
					result, err = s.Version(ctx.Request.Context(), actor, scope, id, version, kind == "catalog-product")
				case "catalog-product:versions":
					result, err = s.Versions(ctx.Request.Context(), actor, scope, id, f)
				case "catalog:changes":
					result, err = s.Changes(ctx.Request.Context(), actor, scope, f)
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
				switch kind + ":" + action {
				case "manufacturer:create":
					var in c.ManufacturerCreate
					if err = h.Decode(raw, &in); err == nil {
						err = c.RejectNull(raw, "name", "description", "code")
					}
					if err == nil {
						result, err = s.CreateManufacturer(ctx.Request.Context(), actor, scope, in, meta)
					}
				case "manufacturer:update":
					var in c.ManufacturerUpdate
					if err = h.Decode(raw, &in); err == nil {
						err = c.RejectNull(raw, "name", "description")
					}
					if err == nil {
						result, err = s.MutateManufacturer(ctx.Request.Context(), actor, scope, id, c.ManufacturerMutation{Action: "update", ManufacturerUpdate: in}, meta)
					}
				case "manufacturer:state":
					var in c.ManufacturerState
					if err = h.Decode(raw, &in); err == nil {
						result, err = s.MutateManufacturer(ctx.Request.Context(), actor, scope, id, c.ManufacturerMutation{Action: "state", ManufacturerUpdate: c.ManufacturerUpdate{RevisionInput: in.RevisionInput}, Status: in.Status}, meta)
					}
				case "manufacturer:delete":
					var in c.RevisionInput
					if err = h.Decode(raw, &in); err == nil {
						result, err = s.MutateManufacturer(ctx.Request.Context(), actor, scope, id, c.ManufacturerMutation{Action: "delete", ManufacturerUpdate: c.ManufacturerUpdate{RevisionInput: in}}, meta)
					}
				case "manufacturer:preview":
					var in c.ManufacturerMutation
					in, err = manufacturerPreviewInput(raw)
					if err == nil {
						result, err = s.PreviewManufacturer(ctx.Request.Context(), actor, scope, id, in)
					}
				case "product:create":
					var in c.ProductCreate
					if err = h.Decode(raw, &in); err == nil {
						err = c.RejectNull(raw, "name", "model", "description", "purpose", "release_notes", "output_slots")
					}
					if err == nil {
						result, err = s.CreateProduct(ctx.Request.Context(), actor, scope, in, meta)
					}
				case "product:copy":
					var in c.ProductCopy
					if err = h.Decode(raw, &in); err == nil {
						err = c.RejectNull(raw, "name", "model")
					}
					if err == nil {
						result, err = s.CopyProduct(ctx.Request.Context(), actor, scope, in, meta)
					}
				case "product:draft-save":
					var in c.DraftSave
					if err = h.Decode(raw, &in); err == nil {
						result, err = s.SaveDraft(ctx.Request.Context(), actor, scope, id, in, meta)
					}
				case "product:draft-create":
					var in c.DraftCreate
					if err = h.Decode(raw, &in); err == nil {
						result, err = s.NewDraft(ctx.Request.Context(), actor, scope, id, in, meta)
					}
				case "product:draft-delete", "product:publish":
					var in c.DraftRevision
					if err = h.Decode(raw, &in); err == nil {
						if action == "publish" {
							result, err = s.Publish(ctx.Request.Context(), actor, scope, id, in, meta)
						} else {
							result, err = s.DeleteDraft(ctx.Request.Context(), actor, scope, id, in, meta)
						}
					}
				case "product:validate":
					var empty struct{}
					if err = h.Decode(raw, &empty); err == nil {
						result, err = s.Validate(ctx.Request.Context(), actor, scope, id)
					}
				case "product:state":
					var in c.VersionState
					if err = h.Decode(raw, &in); err == nil {
						result, err = s.MutateProduct(ctx.Request.Context(), actor, scope, id, c.ProductMutation{Action: "state", VersionState: in, VersionID: version}, meta)
					}
				case "product:delete":
					var in c.RevisionInput
					if err = h.Decode(raw, &in); err == nil {
						result, err = s.MutateProduct(ctx.Request.Context(), actor, scope, id, c.ProductMutation{Action: "delete", VersionState: c.VersionState{RevisionInput: in}}, meta)
					}
				case "product:preview":
					var in c.ProductMutation
					in, err = productPreviewInput(raw)
					if err == nil {
						result, err = s.PreviewProduct(ctx.Request.Context(), actor, scope, id, in)
					}
				default:
					err = h.Invalid("不支持的命令。")
				}
			}
			h.Reply(ctx, 200, result, err)
		})
	}
}
