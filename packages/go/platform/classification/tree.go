package classification

import (
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"slices"
	"strings"
)

// Normalize derives paths solely from stable identity and declared ancestry.
func Normalize(in Content, publication bool) (Content, error) {
	out := in
	out.Nodes = append([]Node{}, in.Nodes...)
	if in.DictionaryCode != DictionaryCode {
		return out, k.Invalid("/dictionary_code", "dictionary", "仅支持 device_category 业务字典。")
	}
	if len(out.Nodes) > MaxCategories {
		return out, k.Problem("classification_budget_exceeded", "分类最多 10,000 项，不能截断保存。", 422)
	}
	nodes := map[string]Node{}
	codes := map[string]bool{}
	for i, n := range out.Nodes {
		path := fmt.Sprintf("/nodes/%d", i)
		n.Name = strings.TrimSpace(n.Name)
		n.Code = strings.TrimSpace(n.Code)
		if !access.ValidID(n.ID) || !k.Code(n.Code) || !slices.Contains([]string{"active", "inactive"}, n.Status) {
			return out, k.Invalid(path, "node", "分类需要有效 ID、稳定编号和状态。")
		}
		if e := k.String(n.Name, path+"/name", 200, true); e != nil {
			return out, e
		}
		if e := k.String(n.Description, path+"/description", 2000, false); e != nil {
			return out, e
		}
		if _, ok := nodes[n.ID]; ok || codes[strings.ToLower(n.Code)] {
			return out, k.Invalid(path, "duplicate", "分类 ID 或编号重复。")
		}
		codes[strings.ToLower(n.Code)] = true
		nodes[n.ID] = n
	}
	if in.SensorRootID != "" {
		if _, ok := nodes[in.SensorRootID]; !ok {
			return out, k.Invalid("/sensor_root_id", "sensor_root", "传感器分支必须引用本字典的稳定分类 ID。")
		}
	} else if publication {
		return out, k.Invalid("/sensor_root_id", "sensor_root", "发布前请明确传感器分支；不能按名称猜测。")
	}
	for i, n := range out.Nodes {
		n = nodes[n.ID]
		path := []string{n.ID}
		names := []string{n.Name}
		seen := map[string]bool{n.ID: true}
		parent := n.ParentID
		for parent != "" {
			v, ok := nodes[parent]
			if !ok {
				return out, k.Invalid("/nodes/parent_id", "parent", "父分类必须属于同一字典草稿。")
			}
			if seen[parent] {
				return out, k.Invalid("/nodes/parent_id", "cycle", "分类不能包含自引用或循环。")
			}
			seen[parent] = true
			path = append(path, parent)
			names = append(names, v.Name)
			if len(path) > MaxDepth {
				return out, k.Problem("classification_budget_exceeded", "分类技术深度最多 128 层。", 422)
			}
			parent = v.ParentID
		}
		slices.Reverse(path)
		slices.Reverse(names)
		n.Path = path
		n.NamePath = names
		n.Sensor = slices.Contains(path, in.SensorRootID)
		out.Nodes[i] = n
	}
	slices.SortFunc(out.Nodes, func(a, b Node) int {
		if a.Order < b.Order {
			return -1
		}
		if a.Order > b.Order {
			return 1
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out, nil
}
