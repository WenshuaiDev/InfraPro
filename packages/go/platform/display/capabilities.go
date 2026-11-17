package display

import (
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"slices"
	"strings"
)

var capabilities = func() map[string]Capability {
	out := map[string]Capability{}
	add := func(name string, sub bool, p ...access.Code) {
		out[name] = Capability{Name: name, Version: 1, Availability: "available", Permissions: p, Subscribe: sub, MaxTargets: 500}
	}
	add("resources.read", false, access.ProjectEngineeringResourceRead)
	add("channels.read", false, access.ProjectEngineeringResourceRead, access.ProjectMonitoringDataRead)
	for _, name := range []string{"latest", "details", "trends", "statistics", "overview"} {
		add("monitoring."+name, name != "details", access.ProjectEngineeringResourceRead, access.ProjectMonitoringDataRead)
	}
	add("sources.read", false, access.ProjectEngineeringResourceHistoryRead, access.ProjectEngineeringResourceRead, access.ProjectMonitoringDataRead, access.ProjectDeviceRead, access.ProjectDeviceHistoryRead)
	add("artifacts.read", false, access.ProjectEngineeringResourceRead, access.ProjectMonitoringDataRead, access.DerivedCode("project", "analysis.results.read"))
	add("spatial.read", false, access.ProjectSpatialView, access.ProjectEngineeringResourceRead)
	add("views.read", false)
	add("queries.read", false)
	for _, name := range []string{"resources.directory", "resources.detail", "statistics.resources"} {
		add(name, name == "statistics.resources", access.ProjectEngineeringResourceRead)
	}
	add("statistics.structure", true, access.ProjectEngineeringResourceRead)
	add("resources.hierarchy", false, access.ProjectEngineeringResourceRead)
	add("classification.categories", false, access.ProjectDeviceRead)
	add("statistics.deviceCategories", true, access.ProjectDeviceRead)
	add("devices.read", false, access.ProjectDeviceRead)
	add("devices.history", false, access.ProjectDeviceRead, access.ProjectDeviceHistoryRead)
	add("sources.directory", false, access.ProjectEngineeringResourceRead, access.ProjectMonitoringDataRead)
	add("sources.history", false, access.ProjectEngineeringResourceRead, access.ProjectMonitoringDataRead, access.ProjectEngineeringResourceHistoryRead)
	add("map.read", false, access.ProjectEngineeringResourceRead)
	add("spatial.catalog", false, access.ProjectSpatialView, access.ProjectEngineeringResourceRead)
	add("spatial.links", false, access.ProjectSpatialView, access.ProjectEngineeringResourceRead)
	add("statistics.devices", true, access.ProjectDeviceRead)
	add("statistics.monitoring", true, access.ProjectEngineeringResourceRead, access.ProjectMonitoringDataRead)
	for name, c := range out {
		c.MaxViewMembers, c.MaxPageSize, c.AsyncDeadlineSeconds = 50000, 200, 120
		out[name] = c
	}
	for _, name := range []string{"query", "summary", "subscribe"} {
		add("alerts."+name, name == "subscribe")
		v := out["alerts."+name]
		v.Availability = "unavailable"
		v.Reason = "告警业务模块尚未接入。"
		out[v.Name] = v
	}
	return out
}()

func Methods() map[string]string {
	out := map[string]string{}
	for k, v := range methodCapabilities {
		out[k] = v
	}
	return out
}
func (s *Service) Capabilities() []Capability {
	out := []Capability{}
	for _, v := range s.descriptors {
		if s.providers[v.Name] == nil {
			v.Availability = "unavailable"
			v.Reason = "业务读取服务未配置。"
		}
		out = append(out, v)
	}
	slices.SortFunc(out, func(a, b Capability) int { return strings.Compare(a.Name, b.Name) })
	return out
}
func (s *Service) permissionsFor(caps []string) []access.Code {
	out := []access.Code{}
	for _, name := range caps {
		out = append(out, s.descriptors[name].Permissions...)
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// Field and relation delegations can require more than a capability's minimum
// operation. This same intersection is used at save, prepare and runtime.
func (s *Service) grantPermissions(g Grant) []access.Code {
	out := s.permissionsFor(g.Capabilities)
	for _, scope := range g.ReadScopes {
		switch scope.Domain {
		case "resources":
			out = append(out, access.ProjectEngineeringResourceRead)
		case "channels":
			out = append(out, access.ProjectEngineeringResourceRead, access.ProjectMonitoringDataRead)
			if slices.Contains(scope.FieldGroups, "history") {
				out = append(out, access.ProjectMonitoringHistoryRead, access.ProjectEngineeringResourceHistoryRead)
			}
		case "devices":
			out = append(out, access.ProjectDeviceRead)
			if slices.Contains(scope.FieldGroups, "history") {
				out = append(out, access.ProjectDeviceHistoryRead)
			}
		case "spatial":
			out = append(out, access.ProjectSpatialView, access.ProjectEngineeringResourceRead)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}
