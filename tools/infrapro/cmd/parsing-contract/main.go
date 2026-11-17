// Command parsing-contract derives parser wire schemas from authoritative Go DTOs.
package main

import (
	"encoding/json"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	s "github.com/WenshuaiDev/InfraPro/packages/go/platform/parsing"
	tr "github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"os"
	"reflect"
	"strings"
	"time"
)

type schema = map[string]any

var schemas = map[string]schema{}
var names = map[reflect.Type]string{}

func register(name string, v any) { names[reflect.TypeOf(v)] = "Parsing" + name }
func value(t reflect.Type) schema {
	if t == reflect.TypeFor[json.RawMessage]() {
		return schema{}
	}
	if t == reflect.TypeFor[time.Time]() {
		return schema{"type": "string", "format": "date-time"}
	}
	if t.Kind() == reflect.Pointer {
		return schema{"anyOf": []any{value(t.Elem()), schema{"type": "null"}}}
	}
	if name, ok := names[t]; ok {
		if _, ok := schemas[name]; !ok {
			schemas[name] = schema{}
			schemas[name] = structure(t)
		}
		return schema{"$ref": "#/components/schemas/" + name}
	}
	switch t.Kind() {
	case reflect.Struct:
		return structure(t)
	case reflect.String:
		return schema{"type": "string"}
	case reflect.Bool:
		return schema{"type": "boolean"}
	case reflect.Int, reflect.Int64, reflect.Int32:
		return schema{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return schema{"type": "number"}
	case reflect.Slice:
		return schema{"anyOf": []any{schema{"type": "array", "items": value(t.Elem())}, schema{"type": "null"}}}
	case reflect.Array:
		return schema{"type": "array", "items": value(t.Elem())}
	case reflect.Map:
		return schema{"anyOf": []any{schema{"type": "object", "additionalProperties": value(t.Elem())}, schema{"type": "null"}}}
	default:
		return schema{}
	}
}
func structure(t reflect.Type) schema {
	props := map[string]any{}
	required := []string{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := strings.Split(f.Tag.Get("json"), ",")
		if tag[0] == "-" || !f.IsExported() {
			continue
		}
		if f.Anonymous {
			sub := structure(f.Type)
			for k, v := range sub["properties"].(map[string]any) {
				props[k] = v
			}
			required = append(required, sub["required"].([]string)...)
			continue
		}
		if tag[0] == "" {
			continue
		}
		props[tag[0]] = value(f.Type)
		if len(tag) == 1 {
			required = append(required, tag[0])
		}
	}
	return schema{"type": "object", "properties": props, "required": required, "additionalProperties": false}
}

type page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor"`
}

func main() {
	types := map[string]any{"Capability": s.Capability{}, "CapabilityPage": page[s.Capability]{}, "CatalogStatus": s.CatalogStatus{}, "Result": s.Result{}, "Method": s.Method{}, "Meta": s.Meta{}, "StateInput": s.StateInput{}, "Binding": s.Binding{}, "Ignored": s.Ignored{}, "Mapping": s.Mapping{}, "Snapshot": s.Snapshot{}, "ConfigInput": s.ConfigInput{}, "ConfigRevision": s.ConfigRevision{}, "Default": s.Default{}, "DefaultInput": s.DefaultInput{}, "Adoption": s.Adoption{}, "DeviceState": s.DeviceState{}, "Command": s.Command{}, "BatchDevice": s.BatchDevice{}, "BatchCommand": s.BatchCommand{}, "Preview": s.Preview{}, "SampleInput": s.SampleInput{}, "Sample": s.Sample{}, "RunInput": s.RunInput{}, "FrozenInput": s.FrozenInput{}, "Run": s.Run{}, "Execution": s.Execution{}, "Attempt": s.Attempt{}, "RunDetail": s.RunDetail{}, "ResultView": s.ResultView{}, "CompareInput": s.CompareInput{}, "CompareResult": s.CompareResult{}, "WireMethod": c.Method{}, "WireSemantics": c.Semantics{}, "WireOutput": c.Output{}, "WireManifest": c.Manifest{}, "WireEntry": c.Entry{}, "WireCatalog": c.Catalog{}, "WireLimits": c.Limits{}, "WireInput": c.Input{}, "WireContext": c.Context{}, "WireRequest": c.Request{}, "WireRange": c.Range{}, "WireLocation": c.Location{}, "WireIdentity": c.Identity{}, "WireField": c.Field{}, "WireMeasurement": c.Measurement{}, "WireRecord": c.Record{}, "WireStep": c.Step{}, "WireDiagnostic": c.Diagnostic{}, "WireCheck": c.Check{}, "WireResponse": c.Response{}, "WireError": c.Error{}, "MethodPage": page[s.Method]{}, "ConfigRevisionPage": page[s.ConfigRevision]{}, "AdoptionPage": page[s.Adoption]{}, "SamplePage": page[s.Sample]{}, "RunPage": page[s.Run]{}}
	names[reflect.TypeFor[tr.Ref]()] = "TaskRef"
	names[reflect.TypeFor[tr.SourceRef]()] = "TaskSourceRef"
	schemas["TaskRef"] = schema{}
	schemas["TaskSourceRef"] = schema{}
	for n, v := range types {
		register(n, v)
	}
	for _, v := range types {
		value(reflect.TypeOf(v))
	}
	policies := []access.Policy{}
	for _, p := range access.Policies() {
		if strings.HasPrefix(string(p.Operation), "Parsing:") {
			policies = append(policies, p)
		}
	}
	delete(schemas, "TaskRef")
	delete(schemas, "TaskSourceRef")
	if e := json.NewEncoder(os.Stdout).Encode(map[string]any{"schemas": schemas, "policies": policies}); e != nil {
		panic(e)
	}
}
