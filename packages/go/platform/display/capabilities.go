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
