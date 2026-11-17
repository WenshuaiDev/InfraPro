// Command spatial-contract derives the spatial wire schema from Go DTOs and policies.
package main

import (
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/location"
	s "github.com/WenshuaiDev/InfraPro/packages/go/platform/spatial"
	"os"
	"reflect"
	"strings"
	"time"
)

type schema = map[string]any

var schemas = map[string]schema{}
var names = map[reflect.Type]string{}

func register(name string, v any) { names[reflect.TypeOf(v)] = "Spatial" + name }
func value(t reflect.Type) schema {
	if t == reflect.TypeFor[json.RawMessage]() {
		return schema{}
	}
	if t == reflect.TypeFor[time.Time]() {
		return schema{"type": "string", "format": "date-time"}
	}
	if t.Kind() == reflect.Pointer {
		return schema{"allOf": []any{value(t.Elem())}, "nullable": true}
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
	case reflect.Slice, reflect.Array:
		return schema{"type": "array", "items": value(t.Elem())}
	case reflect.Map:
		return schema{"type": "object", "additionalProperties": value(t.Elem())}
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
func main() {
	types := map[string]any{"Asset": s.Asset{}, "Version": s.Version{}, "File": s.File{}, "Manifest": s.Manifest{}, "Coordinates": s.Coordinates{}, "Page": s.Page{}, "PageImage": s.PageImage{}, "Element": s.Element{}, "Fragment": s.Fragment{}, "Feature": s.Feature{}, "FeatureView": s.FeatureView{}, "Camera": s.Camera{}, "Settings": s.Settings{}, "Placement": s.Placement{}, "AssociationGeometry": location.AssociationGeometry{}, "ComponentLink": s.ComponentLink{}, "FeatureLink": s.FeatureLink{}, "Candidate": s.Candidate{}, "Draft": s.Draft{}, "ObjectRef": s.ObjectRef{}, "Object": s.Object{}, "AssetInput": s.AssetInput{}, "VersionInput": s.VersionInput{}, "DraftInput": s.DraftInput{}, "ControlInput": s.ControlInput{}, "UploadInput": s.UploadInput{}, "Upload": s.Upload{}, "Job": s.Job{}, "Grant": s.Grant{}, "GrantChange": s.GrantChange{}, "AccessInput": s.AccessInput{}, "AccessDetail": s.AccessDetail{}, "View": s.View{}, "ViewInput": s.ViewInput{}, "RelatedInput": s.RelatedInput{}, "ObjectQuery": s.ObjectQuery{}, "RevisionInput": s.RevisionInput{}, "Result": s.Result{}, "Preview": s.Preview{}, "Issue": s.Issue{}, "Error": s.Error{}, "Location": s.Location{}, "Basemap": s.Basemap{}}
	for n, v := range types {
		register(n, v)
	}
	for _, v := range types {
		value(reflect.TypeOf(v))
	}
	schemas["SpatialAssociationGeometry"]["properties"].(map[string]any)["kind"].(schema)["enum"] = []string{"region", "path", "plane"}
	policies := []access.Policy{}
	for _, p := range access.Policies() {
		if strings.HasPrefix(string(p.Operation), "Spatial:") {
			policies = append(policies, p)
		}
	}
	if e := json.NewEncoder(os.Stdout).Encode(map[string]any{"schemas": schemas, "policies": policies}); e != nil {
		panic(e)
	}
}
