package devicecataloghttp

import (
	"encoding/json"
	c "github.com/WenshuaiDev/InfraPro/packages/go/platform/devicecatalog"
	h "github.com/WenshuaiDev/InfraPro/services/api/internal/transport/http/devicecommon"
)

func previewAction(raw []byte) (string, error) {
	var head struct {
		Action string `json:"action"`
	}
	if json.Unmarshal(raw, &head) != nil {
		return "", h.Invalid("预览输入无效。")
	}
	return head.Action, nil
}
func manufacturerPreviewInput(raw []byte) (c.ManufacturerMutation, error) {
	var out c.ManufacturerMutation
	action, e := previewAction(raw)
	if e != nil {
		return out, e
	}
	switch action {
	case "update":
		var in struct {
			Action string `json:"action"`
			c.ManufacturerUpdate
		}
		if e = h.Decode(raw, &in); e != nil {
			return out, e
		}
		return c.ManufacturerMutation{Action: action, ManufacturerUpdate: in.ManufacturerUpdate}, nil
	case "state":
		var in struct {
			Action string `json:"action"`
			c.ManufacturerState
		}
		if e = h.Decode(raw, &in); e != nil {
			return out, e
		}
		return c.ManufacturerMutation{Action: action, ManufacturerUpdate: c.ManufacturerUpdate{RevisionInput: in.RevisionInput}, Status: in.Status}, nil
	case "delete":
		var in struct {
			Action string `json:"action"`
			c.RevisionInput
		}
		if e = h.Decode(raw, &in); e != nil {
			return out, e
		}
		return c.ManufacturerMutation{Action: action, ManufacturerUpdate: c.ManufacturerUpdate{RevisionInput: in.RevisionInput}}, nil
	}
	return out, h.Invalid("请选择有效的厂家操作。")
}
func productPreviewInput(raw []byte) (c.ProductMutation, error) {
	var out c.ProductMutation
	action, e := previewAction(raw)
	if e != nil {
		return out, e
	}
	switch action {
	case "state":
		var in struct {
			Action    string `json:"action"`
			VersionID string `json:"version_id"`
			c.VersionState
		}
		if e = h.Decode(raw, &in); e != nil {
			return out, e
		}
		return c.ProductMutation{Action: action, VersionID: in.VersionID, VersionState: in.VersionState}, nil
	case "delete":
		var in struct {
			Action string `json:"action"`
			c.RevisionInput
		}
		if e = h.Decode(raw, &in); e != nil {
			return out, e
		}
		return c.ProductMutation{Action: action, VersionState: c.VersionState{RevisionInput: in.RevisionInput}}, nil
	}
	return out, h.Invalid("请选择有效的产品操作。")
}
