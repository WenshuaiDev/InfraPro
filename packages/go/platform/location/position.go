// Package location owns pure position data and validation shared by resources and devices.
package location

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"
)

type LinearReference struct {
	RouteCode     string   `json:"route_code"`
	RouteName     string   `json:"route_name"`
	ReferenceName string   `json:"reference_name"`
	StartStation  string   `json:"start_station"`
	EndStation    string   `json:"end_station"`
	Side          string   `json:"side"`
	Direction     string   `json:"direction"`
	OffsetM       *float64 `json:"offset_m,omitempty"`
	Note          string   `json:"note"`
}
type Geographic struct {
	CRS       string   `json:"crs"`
	Longitude *float64 `json:"longitude"`
	Latitude  *float64 `json:"latitude"`
}
type Projected struct {
	CRS             string   `json:"crs"`
	X               *float64 `json:"x"`
	Y               *float64 `json:"y"`
	AxisDescription string   `json:"axis_description"`
	Unit            string   `json:"unit"`
}
type Elevation struct {
	ValueM *float64 `json:"value_m"`
	Datum  string   `json:"datum"`
}
type Local struct {
	ReferenceDescription string   `json:"reference_description"`
	AxesDescription      string   `json:"axes_description"`
	Unit                 string   `json:"unit"`
	X                    *float64 `json:"x"`
	Y                    *float64 `json:"y"`
	Z                    *float64 `json:"z,omitempty"`
}
type Section struct {
	Kind                 string `json:"kind"`
	Code                 string `json:"code"`
	Name                 string `json:"name"`
	ReferenceDescription string `json:"reference_description"`
}
type Position struct {
	Description      string            `json:"description"`
	LinearReferences []LinearReference `json:"linear_references"`
	Geographic       *Geographic       `json:"geographic,omitempty"`
	Projected        *Projected        `json:"projected,omitempty"`
	Elevation        *Elevation        `json:"elevation,omitempty"`
	Local            *Local            `json:"local,omitempty"`
	Sections         []Section         `json:"sections"`
	SourceReference  string            `json:"source_reference"`
	SourceDate       *string           `json:"source_date,omitempty"`
	AccuracyM        *float64          `json:"accuracy_m,omitempty"`
}

// Shared text/numeric boundary checks include optional coordinates, preserving actual zero.
func positionBoundary(v reflect.Value, path string) error {
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		return positionBoundary(v.Elem(), path)
	}
	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			key := strings.Split(v.Type().Field(i).Tag.Get("json"), ",")[0]
			if e := positionBoundary(v.Field(i), path+"/"+key); e != nil {
				return e
			}
		}
	case reflect.Slice:
		if v.Len() > 20 {
			return invalid(path, "input_limit", "每组最多 20 项。")
		}
		for i := 0; i < v.Len(); i++ {
			if e := positionBoundary(v.Index(i), fmt.Sprintf("%s/%d", path, i)); e != nil {
				return e
			}
		}
	case reflect.String:
		max := 200
		if strings.Contains(path, "description") || strings.HasSuffix(path, "/note") {
			max = 2000
		}
		if strings.HasSuffix(path, "/source_reference") {
			max = 1000
		}
		if utf8.RuneCountInString(v.String()) > max {
			return invalid(path, "input_limit", fmt.Sprintf("最多 %d 字符。", max))
		}
	case reflect.Float64:
		f := v.Float()
		if math.IsNaN(f) || math.IsInf(f, 0) || math.Abs(f) > 9007199254740991.0 {
			return invalid(path, "finite_number", "请填写安全范围内的有限数值。")
		}
	}
	return nil
}
func ValidatePosition(p *Position) error {
	if p == nil {
		return nil
	}
	if len(jsonBytes(p)) > 32*1024 {
		return problem("input_too_large", "位置内容最多 32 KiB。", 413)
	}
	if e := positionBoundary(reflect.ValueOf(p), "/position"); e != nil {
		return e
	}
	required := func(path, s string) error {
		if strings.TrimSpace(s) == "" {
			return invalid("/position/"+path, "required", "请填写明确的参照说明。")
		}
		return nil
	}
	number := func(path string, n *float64) error {
		if n == nil {
			return invalid("/position/"+path, "required", "请填写数值；未知时可清空整组位置。")
		}
		return nil
	}
	unit := func(path, s string) error {
		if s != "m" && s != "mm" {
			return invalid("/position/"+path, "unit", "长度单位须为 m 或 mm。")
		}
		return nil
	}
	for i, r := range p.LinearReferences {
		path := fmt.Sprintf("linear_references/%d/", i)
		if e := required(path+"route_code", r.RouteCode); e != nil {
			return e
		}
		if r.EndStation != "" && strings.TrimSpace(r.StartStation) == "" {
			return invalid("/position/"+path+"start_station", "required", "填写终点桩号时须提供起点桩号。")
		}
	}
	for i, r := range p.Sections {
		path := fmt.Sprintf("sections/%d/", i)
		if r.Kind != "route_cross_section" && r.Kind != "facility_monitoring_section" {
			return invalid("/position/"+path+"kind", "section_kind", "请选择工程断面或设施内部监测断面。")
		}
		if e := required(path+"code", r.Code); e != nil {
			return e
		}
		if e := required(path+"reference_description", r.ReferenceDescription); e != nil {
			return e
		}
	}
	if g := p.Geographic; g != nil {
		if e := required("geographic/crs", g.CRS); e != nil {
			return e
		}
		if e := number("geographic/longitude", g.Longitude); e != nil {
			return e
		}
		if e := number("geographic/latitude", g.Latitude); e != nil {
			return e
		}
		if *g.Longitude < -180 || *g.Longitude > 180 {
			return invalid("/position/geographic/longitude", "range", "经度范围为 -180 至 180。")
		}
		if *g.Latitude < -90 || *g.Latitude > 90 {
			return invalid("/position/geographic/latitude", "range", "纬度范围为 -90 至 90。")
		}
	}
	if g := p.Projected; g != nil {
		for k, v := range map[string]string{"crs": g.CRS, "axis_description": g.AxisDescription} {
			if e := required("projected/"+k, v); e != nil {
				return e
			}
		}
		if e := unit("projected/unit", g.Unit); e != nil {
			return e
		}
		if e := number("projected/x", g.X); e != nil {
			return e
		}
		if e := number("projected/y", g.Y); e != nil {
			return e
		}
	}
	if g := p.Elevation; g != nil {
		if e := required("elevation/datum", g.Datum); e != nil {
			return e
		}
		if e := number("elevation/value_m", g.ValueM); e != nil {
			return e
		}
	}
	if g := p.Local; g != nil {
		for k, v := range map[string]string{"reference_description": g.ReferenceDescription, "axes_description": g.AxesDescription} {
			if e := required("local/"+k, v); e != nil {
				return e
			}
		}
		if e := unit("local/unit", g.Unit); e != nil {
			return e
		}
		if e := number("local/x", g.X); e != nil {
			return e
		}
		if e := number("local/y", g.Y); e != nil {
			return e
		}
	}
	if p.AccuracyM != nil && *p.AccuracyM < 0 {
		return invalid("/position/accuracy_m", "range", "精度数值不能为负。")
	}
	if p.SourceDate != nil {
		if _, e := time.Parse("2006-01-02", *p.SourceDate); e != nil {
			return invalid("/position/source_date", "date", "请填写有效日期。")
		}
	}
	return nil
}

// Issue is independent of storage and authorization.
type Issue struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}
type Error struct {
	Code, Message string
	Status        int
	Issues        []Issue
}

func (e *Error) Error() string { return e.Message }
func problem(code, message string, status int) *Error {
	return &Error{Code: code, Message: message, Status: status, Issues: []Issue{}}
}
func invalid(path, code, message string) error {
	return &Error{Code: "resource_invalid", Message: "请修正标记的问题后重试。", Status: 422, Issues: []Issue{{Path: path, Code: code, Message: message}}}
}
func jsonBytes(v any) []byte { b, _ := json.Marshal(v); return b }
