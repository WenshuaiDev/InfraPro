package runtime

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/display"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/sources"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/spatial"
	"github.com/jackc/pgx/v5"
	"slices"
	"strconv"
	"strings"
)

func installDisplayProviders(b Business) {
	b.Displays.WithBindingQualification(func(ctx context.Context, tx pgx.Tx, subject *access.ReadSubject, bindings map[string]display.Binding) error {
		for _, binding := range bindings {
			if binding.Kind == "artifacts" {
				for _, version := range binding.IDs {
					if e := b.DerivedMonitoring.QualifyArtifactVersionTx(ctx, tx, subject.ReadActor(), derivedmonitoring.ProjectScope(subject.Tenant, subject.Project), version); e != nil {
						return e
					}
				}
			}
		}
		return nil
	})
	p := display.ProviderFunc(func(ctx context.Context, subject *access.ReadSubject, call display.Call) (any, error) {
		q := display.Query{}
		if e := display.Decode(call.Params, &q); e != nil {
			return nil, e
		}
		value, e := b.readDisplay(ctx, subject, call.Method, q)
		if e != nil {
			return nil, e
		}
		return displayProjection(value)
	})
	seen := map[string]bool{}
	for _, cap := range display.Methods() {
		if !seen[cap] && !strings.HasPrefix(cap, "alerts.") {
			if e := b.Displays.Register(cap, p); e != nil {
				panic(e)
			}
			seen[cap] = true
		}
	}
}
func (b Business) readDisplay(ctx context.Context, subject *access.ReadSubject, method string, q display.Query) (any, error) {
	a := subject.ReadActor()
	scope := resources.ProjectScope(subject.Tenant, subject.Project)
	ms := monitoring.ProjectScope(subject.Tenant, subject.Project)
	ids := q.Target.IDs
	switch {
	case strings.HasPrefix(method, "resources."):
		page, e := b.Resources.ReadCatalog(ctx, a, scope, ids, q.Kind, q.Search, q.Cursor, q.Limit)
		if e != nil {
			return nil, e
		}
		if method == "resources.get" {
			if len(page.Items) != 1 {
				return nil, access.ErrNotFound
			}
			return page.Items[0], nil
		}
		if method == "resources.summary" {
			return map[string]any{"count": page.Count, "counts": page.Counts}, nil
		}
		return page, nil
	case strings.HasPrefix(method, "channels."):
		page, e := b.Monitoring.ChannelDirectory(ctx, a, ms, monitoring.ChannelNavigationQuery{IDs: ids, Search: q.Search, Cursor: q.Cursor, Channel: q.ID})
		if e != nil {
			return nil, e
		}
		if method == "channels.get" {
			if len(page.Items) != 1 {
				return nil, access.ErrNotFound
			}
			return page.Items[0], nil
		}
		return page, nil
	case strings.HasPrefix(method, "monitoring."):
		if len(ids) == 0 {
			return map[string]any{"state": "empty", "items": []any{}, "coverage": []any{}}, nil
		}
		selection := q.Selection
		if selection == "" {
			if q.Snapshot != "" || q.Cursor != "" {
				return nil, &display.Error{Code: "query_invalid", Status: 422, Message: "固定快照分页需要原选择句柄。"}
			}
			selected, e := b.Monitoring.Select(ctx, a, ms, monitoring.SelectionInput{Channels: ids})
			if e != nil {
				return nil, e
			}
			selection = selected.ID
		} else if e := b.Monitoring.CheckSelectionTargets(ctx, a, ms, selection, ids); e != nil {
			return nil, e
		}
		query := monitoring.QueryInput{Selection: selection, Snapshot: q.Snapshot, Range: q.Range, ViewRange: q.ViewRange, AllHistory: q.AllHistory, Qualities: q.Qualities, Timezone: q.Timezone, Cursor: q.Cursor, Limit: q.Limit, PlotWidth: q.PlotWidth, PlotBudget: q.PlotBudget, Period: q.Period}
		if query.Timezone == "" {
			query.Timezone = "Asia/Shanghai"
		}
		if method == "monitoring.latest" && len(query.Qualities) == 0 {
			query.Qualities = []string{"good", "unmarked", "suspect", "bad"}
		}
		var value any
		var e error
		if method == "monitoring.details" {
			value, e = b.Monitoring.Details(ctx, a, ms, query)
		} else {
			value, e = b.Monitoring.Analytics(ctx, a, ms, strings.TrimPrefix(method, "monitoring."), query)
		}
		if e != nil {
			return nil, e
		}
		var out map[string]any
		raw, _ := json.Marshal(value)
		d := json.NewDecoder(bytes.NewReader(raw))
		d.UseNumber()
		if e = d.Decode(&out); e != nil {
			return nil, e
		}
		out["selection_id"] = selection
		// Owning-domain coverage distinguishes missing calculated intervals from
		// complete results. Keep its ranges and reasons alongside partial status.
		if out["state"] == "ready" {
			if coverage, ok := out["coverage"].([]any); ok {
				for _, value := range coverage {
					if c, ok := value.(map[string]any); ok && c["state"] != "current" {
						out["state"] = "partial"
						break
					}
				}
			}
		}
		out["filters"] = map[string]any{"range": query.Range, "all_history": query.AllHistory, "qualities": query.Qualities, "timezone": query.Timezone}
		return out, nil
	case strings.HasPrefix(method, "sources."):
		if len(ids) != 1 {
			return nil, &display.Error{Code: "query_invalid", Status: 422, Message: "来源查询需要单个通道。"}
		}
		channels, e := b.Monitoring.ChannelDirectory(ctx, a, ms, monitoring.ChannelNavigationQuery{IDs: ids})
		if e != nil {
			return nil, e
		}
		if len(channels.Items) != 1 || channels.Items[0].Point == nil {
			return nil, access.ErrNotFound
		}
		c := channels.Items[0]
		target := sources.Target{FacilityID: c.Facility, PointID: *c.Point, ChannelID: c.ID}
		ss := sources.Scope{Entry: "project", Tenant: subject.Tenant, Project: subject.Project}
		if method == "sources.get" {
			return b.Sources.Current(ctx, a, ss, target)
		}
		return b.Sources.History(ctx, a, ss, target, sources.Filter{Cursor: q.Cursor, Limit: q.Limit})
	case strings.HasPrefix(method, "artifacts."):
		ds := derivedmonitoring.ProjectScope(subject.Tenant, subject.Project)
		if method == "artifacts.list" {
			items := []derivedmonitoring.AnalysisArtifactDetail{}
			for _, version := range ids {
				if q.Cursor != "" && version <= q.Cursor {
					continue
				}
				id, e := b.DerivedMonitoring.ResolveArtifactVersion(ctx, a, ds, version)
				if e != nil {
					return nil, e
				}
				v, e := b.DerivedMonitoring.AnalysisArtifact(ctx, a, ds, id, version)
				if e != nil {
					return nil, e
				}
				items = append(items, v)
				if len(items) >= 50 {
					break
				}
			}
			cursor := ""
			if len(items) == 50 {
				cursor = items[len(items)-1].Version.ID
			}
			return map[string]any{"items": items, "next_cursor": cursor}, nil
		}
		if len(ids) != 1 {
			return nil, &display.Error{Code: "query_invalid", Status: 422, Message: "成果读取需要固定版本。"}
		}
		artifact, e := b.DerivedMonitoring.ResolveArtifactVersion(ctx, a, ds, ids[0])
		if e != nil {
			return nil, e
		}
		if method == "artifacts.get" {
			return b.DerivedMonitoring.AnalysisArtifact(ctx, a, ds, artifact, ids[0])
		}
		query := derivedmonitoring.AnalysisArtifactQuery{VersionID: ids[0], Cursor: q.Cursor, Limit: q.Limit}
		if q.Output != "" {
			query.Outputs = []string{q.Output}
		}
		return b.DerivedMonitoring.AnalysisArtifactRows(ctx, a, ds, artifact, query)
	case strings.HasPrefix(method, "spatial."):
		ss := spatial.Project(subject.Tenant, subject.Project)
		if method == "spatial.list" {
			items := []map[string]any{}
			for _, id := range ids {
				v, e := b.Spatial.Manifest(ctx, a, ss, id)
				if e != nil {
					return nil, e
				}
				items = append(items, map[string]any{"id": v.ID, "asset_id": v.AssetID, "number": strconv.Itoa(v.Number), "format": v.Format, "state": v.State, "revision": v.Revision, "manifest_hash": v.ManifestHash})
			}
			return map[string]any{"items": items}, nil
		}
		if len(ids) != 1 {
			return nil, &display.Error{Code: "query_invalid", Status: 422, Message: "空间读取需要固定版本。"}
		}
		switch method {
		case "spatial.manifest":
			v, e := b.Spatial.Manifest(ctx, a, ss, ids[0])
			if e != nil {
				return nil, e
			}
			return map[string]any{"version_id": v.ID, "revision": v.Revision, "manifest_hash": v.ManifestHash, "manifest": v.Manifest}, nil
		case "spatial.objects":
			return b.Spatial.Elements(ctx, a, ss, ids[0], spatial.Filter{Query: q.Search, Cursor: q.Cursor, Limit: q.Limit})
		case "spatial.placements":
			return b.Spatial.Placements(ctx, a, ss, ids[0], spatial.Filter{Cursor: q.Cursor, Limit: q.Limit})
		case "spatial.readFile":
			d, e := b.Spatial.OpenFile(ctx, a, ss, ids[0], q.FileID, false)
			if e != nil {
				return nil, e
			}
			defer d.Close()
			length := q.Length
			if length == 0 {
				length = min(4<<20, d.Size-q.Offset)
			}
			if length < 0 || q.Offset > d.Size {
				return nil, &display.Error{Code: "query_invalid", Status: 422, Message: "文件分块范围无效。"}
			}
			var data bytes.Buffer
			e = d.Stream(ctx, q.Offset, length, func(chunk []byte) error { _, e := data.Write(chunk); return e })
			if e != nil {
				return nil, e
			}
			return map[string]any{"file_id": q.FileID, "media_type": d.MediaType, "size": strconv.FormatInt(d.Size, 10), "offset": strconv.FormatInt(q.Offset, 10), "length": strconv.FormatInt(length, 10), "sha256": d.SHA256, "base64": base64.StdEncoding.EncodeToString(data.Bytes())}, nil
		}
	}
	return nil, &display.Error{Code: "capability_unavailable", Status: 409, Message: "业务读取能力未接入。"}
}

// Marshal typed integers directly, then preserve their decimal text. This never
// round-trips a 64-bit count through float64. Geometry stays numeric.
func displayProjection(v any) (any, error) {
	raw, e := json.Marshal(v)
	if e != nil {
		return nil, e
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var out any
	if e = d.Decode(&out); e != nil {
		return nil, e
	}
	var walk func(any, string) any
	walk = func(v any, key string) any {
		switch x := v.(type) {
		case map[string]any:
			for k, v := range x {
				x[k] = walk(v, k)
			}
		case []any:
			for i, v := range x {
				x[i] = walk(v, key)
			}
		case json.Number:
			if slices.Contains([]string{"size", "number", "revision", "version_no", "row_count", "count", "output_rows", "point_count"}, key) || strings.HasSuffix(key, "_count") {
				return x.String()
			}
		}
		return v
	}
	return walk(out, ""), nil
}
