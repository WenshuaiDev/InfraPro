// Command parsing-contract derives parser wire schemas from authoritative Go DTOs.
package main

import (
	"encoding/json"
	i "github.com/WenshuaiDev/InfraPro/packages/go/foundation/ingestioncontract"
	r "github.com/WenshuaiDev/InfraPro/packages/go/foundation/relaycontract"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	s "github.com/WenshuaiDev/InfraPro/packages/go/platform/ingestion"
	tr "github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"os"
	"reflect"
	"slices"
	"strings"
	"time"
)

type schema = map[string]any

var schemas = map[string]schema{}
var names = map[reflect.Type]string{}

func register(name string, v any) { names[reflect.TypeOf(v)] = "Ingestion" + name }
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
		if slices.Contains(tag, "string") {
			props[tag[0]] = schema{"type": "string", "pattern": "^[0-9]+$"}
		}
		if len(tag) == 1 {
			required = append(required, tag[0])
		}
	}
	return schema{"type": "object", "properties": props, "required": required, "additionalProperties": false}
}

type page[T any] struct {
	Total      *int   `json:"total,omitempty"`
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor"`
}

func main() {
	types := map[string]any{
		"Result": s.Result{}, "Meta": s.Meta{}, "Source": s.Source{}, "SourcePage": page[s.Source]{}, "SourceRevision": s.SourceRevision{}, "SourceDetail": s.SourceDetail{}, "SourceInput": s.SourceInput{}, "Command": s.Command{}, "RelayInput": s.RelayInput{}, "Terminal": s.Terminal{}, "TerminalPage": page[s.Terminal]{}, "TerminalInput": s.TerminalInput{}, "Association": s.Association{}, "AssociationInput": s.AssociationInput{}, "Grant": s.Grant{}, "GrantInput": s.GrantInput{}, "Receipt": s.Receipt{}, "Unit": s.Unit{}, "Decision": s.Decision{}, "Plan": s.Plan{}, "PlanSnapshot": s.PlanSnapshot{}, "Message": s.Message{}, "MetadataPage": page[json.RawMessage]{}, "ResultView": s.ResultView{}, "Recovery": s.Recovery{}, "RecoveryInput": s.RecoveryInput{},
		"IdentityMethod": i.Method{}, "Identification": i.Identification{}, "Protocol": i.Protocol{}, "Compatibility": i.Compatibility{}, "Catalog": i.Catalog{}, "IdentifyRequest": i.Request{}, "IdentifyResponse": i.Response{}, "View": i.View{}, "Transform": i.Transform{}, "IdentifiedUnit": i.Unit{}, "Identity": i.Identity{},
		"RelaySource": r.Source{}, "RelayApply": r.Apply{}, "RelayApplied": r.Applied{}, "Envelope": r.Envelope{}, "Pull": r.Pull{}, "Delivery": r.Delivery{}, "Ack": r.Ack{}, "Acks": r.Acks{}, "AckResults": r.AckResults{}, "Release": r.Release{}, "Releases": r.Releases{}, "Watermark": r.Watermark{}, "WatermarkRequest": r.WatermarkRequest{},
	}
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
		if strings.HasPrefix(string(p.Operation), "Ingestion:") {
			policies = append(policies, p)
		}
	}
	delete(schemas, "TaskRef")
	delete(schemas, "TaskSourceRef")
	if e := json.NewEncoder(os.Stdout).Encode(map[string]any{"schemas": schemas, "policies": policies}); e != nil {
		panic(e)
	}
}
