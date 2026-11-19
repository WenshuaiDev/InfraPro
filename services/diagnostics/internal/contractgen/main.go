// Command contractgen emits reflection-backed input and safe-view schemas.
package main

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/foundation/diagnosticreport"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/access"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/ingest"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/model"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/state"
	api "github.com/WenshuaiDev/InfraPro/services/diagnostics/internal/transport/http"
	"github.com/WenshuaiDev/InfraPro/services/diagnostics/settings"
)

type schema = map[string]any

func shape(t reflect.Type) schema {
	if t == reflect.TypeFor[time.Time]() {
		return schema{"type": "string", "format": "date-time"}
	}
	if t == reflect.TypeFor[json.RawMessage]() {
		return schema{"type": []string{"string", "number", "boolean", "null"}}
	}
	if t == reflect.TypeFor[settings.Binding]() {
		return bindingSchema()
	}
	switch t.Kind() {
	case reflect.Pointer:
		return schema{"anyOf": []any{shape(t.Elem()), schema{"type": "null"}}}
	case reflect.String:
		return schema{"type": "string"}
	case reflect.Bool:
		return schema{"type": "boolean"}
	case reflect.Int, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint32, reflect.Uint64:
		return schema{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return schema{"type": "number"}
	case reflect.Interface:
		return schema{"type": []string{"string", "number", "boolean", "null"}}
	case reflect.Slice, reflect.Array:
		return schema{"type": "array", "items": shape(t.Elem())}
	case reflect.Map:
		return schema{"type": "object", "additionalProperties": shape(t.Elem())}
	case reflect.Struct:
		properties := schema{}
		required := []string{}
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if !field.IsExported() {
				continue
			}
			tag := field.Tag.Get("json")
			if tag == "-" {
				continue
			}
			name, option, _ := strings.Cut(tag, ",")
			if field.Anonymous && name == "" {
				nested := shape(field.Type)
				for k, v := range nested["properties"].(schema) {
					properties[k] = v
				}
				required = append(required, nested["required"].([]string)...)
				continue
			}
			if name == "" {
				name = field.Name
			}
			v := shape(field.Type)
			constrain(t.Name(), name, v)
			properties[name] = v
			if option != "omitempty" {
				required = append(required, name)
			}
		}
		return schema{"type": "object", "additionalProperties": false, "properties": properties, "required": required}
	default:
		panic("unsupported schema type")
	}
}
func enum(s schema, values ...string) { s["enum"] = values }
func constrain(owner, key string, s schema) {
	if s["type"] == "string" {
		switch key {
		case "id", "instance_id", "node_id", "replica_id", "component_id", "binding_id", "probe_id", "key_id", "target_id", "parent_node_id", "from_replica_id", "to_component_id", "vantage_node_id", "dimension_id":
			s["pattern"] = `^[a-z][a-z0-9_.-]{0,63}$`
		case "revision", "assignment_revision", "manifest_revision", "credential_generation", "generation", "epoch", "sequence", "report_seq", "os_start_time_ms", "uptime_ms", "evicted_count":
			s["pattern"] = `^(0|[1-9][0-9]{0,19})$`
		case "boot_id", "observer_boot_id", "server_boot_id", "request_id":
			s["format"] = "uuid"
		case "display_name", "expectation_reason":
			s["minLength"] = 1
			s["maxLength"] = 80
		case "build_id":
			s["maxLength"] = 64
		case "reason_code":
			codes := []string{}
			for c := range settings.Reasons {
				codes = append(codes, c)
			}
			sort.Strings(codes)
			s["enum"] = codes
		}
	}
	switch key {
	case "schema_version", "protocol_major":
		s["const"] = 1
	case "expectation", "default_expectation":
		enum(s, "continuous", "on_demand", "disabled", "external")
	case "resource_scope", "scope":
		enum(s, "host", "vm", "container", "none")
	case "facet":
		enum(s, "presence", "readiness", "loop", "resource")
	case "target_kind":
		enum(s, "node", "replica", "endpoint", "dependency")
	case "presence":
		enum(s, "running", "stopped", "missing", "unknown", "not_applicable")
	case "readiness":
		enum(s, "ready", "not_ready", "starting", "unconfigured", "unknown", "not_applicable")
	case "loop":
		enum(s, "idle", "working", "blocked", "stalled", "unknown", "not_applicable")
	case "freshness":
		enum(s, "fresh", "stale", "never", "historical", "clock_uncertain")
	case "outcome":
		enum(s, "pending", "ok", "failed", "timeout", "permission_denied", "unsupported", "not_configured", "invalid_report")
	case "display_state":
		enum(s, "healthy", "degraded", "unavailable", "unknown", "disabled", "idle")
	case "adapter":
		values := []string{}
		for a := range settings.BindingFields {
			values = append(values, a)
		}
		sort.Strings(values)
		s["enum"] = values
	case "interval_ms":
		s["minimum"] = 1000
		s["maximum"] = 300000
	case "timeout_ms":
		s["minimum"] = 100
		s["maximum"] = 5000
	case "stale_after_ms":
		s["minimum"] = 3000
		s["maximum"] = 900000
	case "availability":
		enum(s, "available", "unlimited", "unsupported", "permission_denied", "not_configured", "error")
	case "evidence_mode":
		enum(s, "self_report", "shared_network", "external_probe")
	case "purpose":
		enum(s, "liveness", "readiness", "serving", "storage", "asset", "control_plane")
	case "source":
		enum(s, "os", "systemd", "docker", "protocol", "self_report")
	case "pid_namespace":
		enum(s, "host", "container")
	case "phase":
		enum(s, "starting", "ready", "unconfigured", "stopping", "failed")
	}
	if key == "kind" {
		switch owner {
		case "Node", "NodeView":
			enum(s, "physical", "vm", "container", "logical_test", "external")
		case "Component", "ComponentView", "CatalogEntry":
			enum(s, "application", "worker", "database", "cache", "object_store", "gateway", "tool", "diagnostics")
		case "Credential":
			enum(s, "password", "bearer", "s3")
		}
	}
	if owner == "ProcessMetric" {
		switch key {
		case "key":
			enum(s, diagnosticreport.MetricKeys()...)
		case "value":
			s["minimum"] = 0
		case "dimension_id":
			s["pattern"] = `^tenant-[0-9a-f]{24}$`
		}
	}
	if owner == "Report" && key == "metrics" {
		s["maxItems"] = diagnosticreport.MaxProcessMetrics
	}
	if owner == "Metric" && key == "key" {
		keys := []string{}
		for key := range model.MetricUnits {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		s["enum"] = keys
	}
	if key == "state" && owner == "Lane" {
		enum(s, "idle", "working", "blocked")
	}
	if key == "state" && owner == "Coverage" {
		enum(s, "complete", "partial", "none")
	}
}
func bindingSchema() schema {
	t := reflect.TypeFor[settings.Binding]()
	props := schema{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		props[name] = shape(f.Type)
	}
	names := []string{}
	for n := range settings.BindingFields {
		names = append(names, n)
	}
	sort.Strings(names)
	branches := []any{}
	required := map[string][]string{"native_process": {"allowed_root", "expected_executable", "expected_uid"}, "self_report": {"report_file", "allowed_root", "expected_replica_id", "expected_executable", "expected_uid", "pid_namespace"}, "host_resources": {"scope"}, "systemd_unit": {"unit_name"}, "docker_container": {"socket_path", "selector_labels", "expected_replica_id"}, "http_live": {"url", "expected_status"}, "http_ready": {"url", "expected_status"}, "tcp": {"host", "port"}, "postgres": {"host", "port", "database", "username", "tls_mode"}, "redis": {"address", "database"}, "clickhouse": {"address", "database", "username"}, "s3": {"endpoint", "bucket"}, "keeper_live": {"host", "port"}, "jetstream_health": {"host", "port"}}
	for _, name := range names {
		p := schema{"id": schema{"type": "string", "pattern": `^[a-z][a-z0-9_.-]{0,63}$`}, "adapter": schema{"type": "string", "const": name}}
		for _, key := range settings.BindingFields[name] {
			p[key] = props[key]
		}
		if name == "redis" {
			p["database"] = schema{"type": "integer", "minimum": 0, "maximum": 15}
		}
		req := append([]string{"id", "adapter"}, required[name]...)
		branches = append(branches, schema{"type": "object", "properties": p, "required": req, "additionalProperties": false})
	}
	return schema{"oneOf": branches}
}
func main() {
	types := map[string]reflect.Type{"Manifest": reflect.TypeFor[settings.Manifest](), "ServerConfig": reflect.TypeFor[settings.Server](), "ObserverConfig": reflect.TypeFor[settings.Observer](), "Bindings": reflect.TypeFor[settings.Bindings](), "Auth": reflect.TypeFor[settings.Auth](), "Keyring": reflect.TypeFor[settings.Keyring](), "ObserverKey": reflect.TypeFor[settings.ObserverKey](), "Credential": reflect.TypeFor[settings.Credential](), "ProcessReport": reflect.TypeFor[diagnosticreport.Report](), "Observation": reflect.TypeFor[model.Observation](), "ObserverSessionRequest": reflect.TypeFor[ingest.SessionRequest](), "ObserverSession": reflect.TypeFor[ingest.SessionResponse](), "ObserverFrame": reflect.TypeFor[ingest.Frame](), "ObserverAck": reflect.TypeFor[ingest.Ack](), "Meta": reflect.TypeFor[api.Meta](), "Bootstrap": reflect.TypeFor[api.Bootstrap](), "LoginRequest": reflect.TypeFor[api.LoginRequest](), "Session": reflect.TypeFor[access.SessionView](), "Activity": reflect.TypeFor[api.Activity](), "Overview": reflect.TypeFor[api.Overview](), "NodeView": reflect.TypeFor[state.NodeView](), "NodeDetail": reflect.TypeFor[api.NodeDetail](), "ComponentView": reflect.TypeFor[state.ComponentView](), "ComponentDetail": reflect.TypeFor[api.ComponentDetail](), "ReplicaView": reflect.TypeFor[state.ReplicaView](), "Check": reflect.TypeFor[state.Check](), "Transition": reflect.TypeFor[model.Transition](), "Metric": reflect.TypeFor[model.Metric](), "MetricSeries": reflect.TypeFor[api.MetricSeries](), "Coverage": reflect.TypeFor[state.Coverage](), "Retention": reflect.TypeFor[api.Retention]()}
	out := map[string]any{"catalog": settings.RuntimeCatalog}
	schemas := schema{}
	for name, t := range types {
		schemas[name] = shape(t)
	}
	out["schemas"] = schemas
	if json.NewEncoder(os.Stdout).Encode(out) != nil {
		os.Exit(1)
	}
}
