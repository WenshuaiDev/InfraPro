// Package devicecommon owns only strict transport handling, not business commands.
package devicecommon

import (
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	identityhttp "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/identity"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"slices"
	"strconv"
)

func Invalid(message string) *c.Error {
	return &c.Error{Code: "invalid_request", Message: message, Status: 400, Issues: []c.Issue{}}
}
func Reply(ctx *gin.Context, status int, value any, err error) {
	if err != nil {
		var e *c.Error
		if errors.As(err, &e) {
			if e.Issues == nil {
				e.Issues = []c.Issue{}
			}
			ctx.AbortWithStatusJSON(e.Status, e)
			return
		}
		identityhttp.RespondError(ctx, err)
		return
	}
	ctx.JSON(status, value)
}
func Body(ctx *gin.Context) ([]byte, error) {
	b, e := io.ReadAll(http.MaxBytesReader(ctx.Writer, ctx.Request.Body, 256*1024))
	if e != nil {
		var large *http.MaxBytesError
		if errors.As(e, &large) {
			return nil, &c.Error{Code: "input_too_large", Message: "单次请求最多 256 KiB。", Status: 413}
		}
		return nil, Invalid("请求读取失败。")
	}
	return b, nil
}
func Decode(raw []byte, value any) error { return c.DecodeRequest(raw, value) }
func Meta(ctx *gin.Context) access.Meta {
	return access.Meta{ActorType: "account", IP: ctx.ClientIP(), UserAgent: ctx.Request.UserAgent(), RequestID: ctx.Writer.Header().Get("X-Request-ID")}
}
func Filter(ctx *gin.Context, allowed ...string) (c.Filter, error) {
	f := c.Filter{Query: ctx.Query("q"), Cursor: ctx.Query("cursor"), ManufacturerID: ctx.Query("manufacturer_id"), ProductID: ctx.Query("product_id"), ProductVersionID: ctx.Query("product_version_id"), DeviceID: ctx.Query("device_id"), Status: ctx.Query("status"), Source: ctx.Query("source"), State: ctx.Query("state"), SlotKey: ctx.Query("slot_key"), Kind: ctx.Query("kind"), Installation: ctx.Query("installation"), MetricID: ctx.Query("metric_version_id"), UnitID: ctx.Query("unit_version_id")}
	f.Level, f.ParentID = ctx.Query("level"), ctx.Query("parent_id")
	for key, values := range ctx.Request.URL.Query() {
		if len(values) != 1 || !slices.Contains(allowed, key) {
			return f, Invalid("不支持或重复的查询参数。")
		}
	}
	if v := ctx.Query("limit"); v != "" {
		n, e := strconv.Atoi(v)
		if e != nil || n < 1 || n > 100 {
			return f, Invalid("分页大小须为 1–100。")
		}
		f.Limit = n
	}
	if v := ctx.Query("compatible_only"); v != "" {
		if v != "true" && v != "false" {
			return f, Invalid("兼容性筛选值无效。")
		}
		b := v == "true"
		f.CompatibleOnly = &b
	}
	return f, nil
}

func PathIDs(ctx *gin.Context) error {
	for _, p := range ctx.Params {
		if !access.ValidID(p.Value) {
			return &c.Error{Code: "device_not_found", Message: "对象不存在或不可访问。", Status: 404}
		}
	}
	return nil
}
