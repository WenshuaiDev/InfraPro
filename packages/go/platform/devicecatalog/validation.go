package devicecatalog

import (
	"context"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
	"slices"
)

func Normalize(c *Content) error {
	c.Name = k.Text(c.Name)
	c.Model = k.Text(c.Model)
	c.Description = k.Text(c.Description)
	c.Purpose = k.Text(c.Purpose)
	c.ReleaseNotes = k.Text(c.ReleaseNotes)
	if c.SchemaVersion != 1 {
		return k.Invalid("/content/schema_version", "schema_version", "只支持格式版本 1。")
	}
	for _, v := range []struct {
		s, path  string
		max      int
		required bool
	}{{c.Name, "name", 200, true}, {c.Model, "model", 200, true}, {c.Description, "description", 2000, false}, {c.Purpose, "purpose", 2000, false}, {c.ReleaseNotes, "release_notes", 2000, false}} {
		if e := k.String(v.s, "/content/"+v.path, v.max, v.required); e != nil {
			return e
		}
	}
	if len(c.OutputSlots) > 256 {
		return k.Invalid("/content/output_slots", "input_limit", "单产品最多 256 个输出槽位。")
	}
	if c.OutputSlots == nil {
		c.OutputSlots = []Slot{}
	}
	keys := map[string]bool{}
	for i := range c.OutputSlots {
		s := &c.OutputSlots[i]
		path := fmt.Sprintf("/content/output_slots/%d", i)
		s.Name = k.Text(s.Name)
		s.Description = k.Text(s.Description)
		s.AggregationDescription = k.Text(s.AggregationDescription)
		if !k.Key(s.Key) || keys[s.Key] {
			return k.Invalid(path+"/key", "slot_key", "键须唯一，以小写字母开头，只含小写字母、数字和下划线，最多 64 位。")
		}
		keys[s.Key] = true
		if e := k.String(s.Name, path+"/name", 200, true); e != nil {
			return e
		}
		if e := k.String(s.Description, path+"/description", 2000, false); e != nil {
			return e
		}
		switch s.Mode {
		case "fixed":
			if len(s.AllowedShapes) > 0 {
				return k.Invalid(path, "mode_fields", "固定输出不能填写可配置形态。")
			}
			if e := validateSemanticFields(Semantics{MetricVersionID: s.MetricVersionID, UnitVersionID: s.UnitVersionID, TimestampMeaning: s.TimestampMeaning, AggregationDescription: s.AggregationDescription}, path); e != nil {
				return e
			}
		case "configurable":
			if s.MetricVersionID != "" || s.UnitVersionID != nil || s.TimestampMeaning != "" || s.AggregationDescription != "" {
				return k.Invalid(path, "mode_fields", "可配置槽位不能填写固定核心含义。")
			}
			if len(s.AllowedShapes) == 0 || len(s.AllowedShapes) > 3 {
				return k.Invalid(path+"/allowed_shapes", "shapes", "请选择允许的值形态。")
			}
			seen := map[string]bool{}
			for _, shape := range s.AllowedShapes {
				if !slices.Contains([]string{"integer", "decimal", "enum"}, shape) || seen[shape] {
					return k.Invalid(path+"/allowed_shapes", "shapes", "值形态无效或重复。")
				}
				seen[shape] = true
			}
			slices.Sort(s.AllowedShapes)
		default:
			return k.Invalid(path+"/mode", "mode", "请选择固定或可配置输出。")
		}
	}
	// Leave enough room for write metadata; no saved draft can exceed publication's input boundary.
	if len(k.JSON(c)) > 250*1024 {
		return k.Problem("input_too_large", "产品正文最多 250 KiB（请求总量最多 256 KiB）。", 413)
	}
	return nil
}
func validateSemanticFields(s Semantics, path string) error {
	if !access.ValidID(s.MetricVersionID) || s.UnitVersionID != nil && !access.ValidID(*s.UnitVersionID) {
		return k.Invalid(path, "definition_id", "请选择有效的固定指标及单位版本。")
	}
	if s.TimestampMeaning != "observation_time" && s.TimestampMeaning != "interval_end" {
		return k.Invalid(path+"/timestamp_meaning", "timestamp_meaning", "请选择观测时刻或统计区间结束时刻。")
	}
	for _, v := range []struct{ key, value string }{{"aggregation_description", s.AggregationDescription}, {"observation_direction", s.ObservationDirection}, {"reference_basis", s.ReferenceBasis}} {
		if e := k.String(v.value, path+"/"+v.key, 2000, s.TimestampMeaning == "interval_end" && v.key == "aggregation_description"); e != nil {
			return e
		}
	}
	return nil
}

// ResolveSemanticsTx resolves fixed public engineering versions under the caller's
// engineering catalog lock. Authorization and adoption intent belong to the caller.
func ResolveSemanticsTx(ctx context.Context, tx pgx.Tx, s Semantics, newUse bool) (Semantics, []engineering.Version, error) {
	s.Normalize()
	defs := []engineering.Version{}
	if e := validateSemanticFields(s, ""); e != nil {
		return s, defs, e
	}
	metric, deps, e := engineering.ResolvePublicTx(ctx, tx, s.MetricVersionID, engineering.MetricDefinition, newUse)
	if e != nil {
		return s, defs, e
	}
	if metric.Content.Metric == nil {
		return s, defs, k.Invalid("/metric_version_id", "metric", "所选版本不是指标。")
	}
	m := metric.Content.Metric
	s.ValueShape = m.Shape
	defs = append(defs, metric)
	defs = append(defs, deps...)
	if m.Shape == "enum" {
		if s.UnitVersionID != nil {
			return s, defs, k.Invalid("/unit_version_id", "enum_unit", "枚举指标不使用单位。")
		}
	} else {
		if s.UnitVersionID == nil || !slices.Contains(m.AllowedUnits, *s.UnitVersionID) {
			return s, defs, k.Invalid("/unit_version_id", "unit", "单位不在指标允许的固定版本集合中。")
		}
		unit, _, e := engineering.ResolvePublicTx(ctx, tx, *s.UnitVersionID, engineering.UnitDefinition, newUse)
		if e != nil {
			return s, defs, e
		}
		if unit.Content.Unit == nil || unit.Content.Unit.Category != m.Category {
			return s, defs, k.Invalid("/unit_version_id", "dimension", "指标与单位的量纲不同。")
		}
		defs = append(defs, unit)
	}
	return s, defs, nil
}
func fixedSemantic(s Slot) Semantics {
	return Semantics{MetricVersionID: s.MetricVersionID, UnitVersionID: s.UnitVersionID, TimestampMeaning: s.TimestampMeaning, AggregationDescription: s.AggregationDescription}
}
func validateTx(ctx context.Context, tx pgx.Tx, auth k.Authorization, product string, c Content, previous *Content, newUse bool) ([]Issue, []engineering.Version, error) {
	issues := []Issue{}
	defs := []engineering.Version{}
	existing := map[string]bool{}
	if previous != nil {
		for _, s := range previous.OutputSlots {
			existing[s.MetricVersionID] = true
			if s.UnitVersionID != nil {
				existing[*s.UnitVersionID] = true
			}
		}
	}
	canRead := auth.Has(access.EngineeringMetricRead) || auth.Has(access.TenantEngineeringCatalogRead)
	for i, s := range c.OutputSlots {
		path := fmt.Sprintf("/content/output_slots/%d", i)
		var mode, signature string
		e := tx.QueryRow(ctx, `SELECT mode,core_signature FROM device_product_slot_keys WHERE product_id=$1 AND key=$2`, product, s.Key).Scan(&mode, &signature)
		if e != nil && e != pgx.ErrNoRows {
			return nil, nil, e
		}
		if e == nil && (mode != s.Mode || signature != s.Core()) {
			issues = append(issues, Issue{Path: path + "/key", Code: "slot_semantics_changed", Message: "此键已经发布过其他测量含义，请使用新键。"})
		}
		if s.Mode != "fixed" {
			continue
		}
		if !canRead && (!existing[s.MetricVersionID] || s.UnitVersionID != nil && !existing[*s.UnitVersionID]) {
			issues = append(issues, Issue{Path: path, Code: "definition_not_found", Message: "所选定义不可访问。"})
			continue
		}
		_, d, e := ResolveSemanticsTx(ctx, tx, fixedSemantic(s), newUse)
		if e != nil {
			if de, ok := k.PublicError(e).(*k.Error); ok {
				if len(de.Issues) > 0 {
					for _, issue := range de.Issues {
						issue.Path = path + issue.Path
						issues = append(issues, issue)
					}
				} else {
					issues = append(issues, Issue{Path: path, Code: de.Code, Message: de.Message})
				}
				continue
			}
			return nil, nil, e
		}
		defs = append(defs, d...)
	}
	return issues, defs, nil
}
