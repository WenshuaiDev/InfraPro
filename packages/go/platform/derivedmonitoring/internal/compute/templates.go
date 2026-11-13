package compute

import "fmt"

type Template struct {
	Key, Version, Name, Description string
	InputCount                      int
}

var templates = []Template{
	{"baseline_delta", "1", "基线变化", "相对明确固定基准的变化，基准改变需要新的输出身份或历史修正。", 1},
	{"difference", "1", "相邻变化", "按实际相邻有效样本求差，默认不跨无效样本。", 1},
	{"rate", "1", "变化速率", "按实际时间差计算；单位时间可选秒、分、小时或固定日。", 1},
	{"subtract", "1", "两通道差值", "按明确时间匹配求差，要求输入的单位、方向与基准兼容。", 2},
	{"window_mean", "1", "滚动均值", "固定时长内等权样本均值，窗口左开右闭。", 1},
	{"window_stddev", "1", "滚动标准差", "总体标准差，保留精确中间方差。", 1},
	{"window_range", "1", "滚动极差", "窗口最大值减最小值。", 1},
	{"weighted_mean", "1", "加权均值", "必需输入全部匹配，非负权重且总和大于零。", 2},
}

func Templates() []Template { return append([]Template(nil), templates...) }

// ExpandTemplate requires explicitly declared source semantics. It never
// guesses affine/ratio scale, direction or baseline from a metric's name.
func ExpandTemplate(key string, inputs []Input) (Rule, error) {
	var chosen *Template
	for i := range templates {
		if templates[i].Key == key {
			chosen = &templates[i]
			break
		}
	}
	if chosen == nil {
		return Rule{}, fmt.Errorf("template_not_found")
	}
	if len(inputs) != chosen.InputCount {
		return Rule{}, fmt.Errorf("template_input_count_mismatch")
	}
	node := Node{ID: "result", Op: key, Inputs: []string{}}
	for _, in := range inputs {
		node.Inputs = append(node.Inputs, "input:"+in.Key)
	}
	rule := Rule{SchemaVersion: SchemaVersion, Inputs: append([]Input(nil), inputs...), Clock: Clock{Kind: "primary_input", Input: inputs[0].Key}, Parameters: []Parameter{}, Outputs: []Output{{Key: "value", Node: "result", SemanticKey: key, Name: chosen.Name, Numeric: NumericContract{Scale: 18, Rounding: "half_even"}}}, Policies: Policies{Quality: []string{"good", "unmarked"}, ConfigBoundary: "break", AllowedLatenessNS: "0"}}
	switch key {
	case "baseline_delta":
		q := inputs[0].Quantity
		rule.Parameters = []Parameter{{Key: "baseline", Type: "decimal", Default: "0", Quantity: &q, IdentityAffecting: true}}
		node.Params.Baseline = &Baseline{Kind: "constant", Value: "param:baseline", Quantity: &q}
	case "difference", "rate":
		node.Params.DifferenceMode = "adjacent"
		node.Params.MaxGapNS = "1800000000000"
		node.Params.InvalidPolicy = "break"
		if key == "rate" {
			node.Params.RateTimeUnit = "minute"
		}
	case "subtract":
		node.Params.Alignment = "exact"
	case "weighted_mean":
		node.Params.Alignment = "exact"
		node.Params.Weights = []string{"1", "1"}
	default:
		node.Params.Window = &Window{Kind: "trailing_duration", DurationNS: "1800000000000"}
		node.Params.MinSamples = "1"
		node.Params.MaxGapNS = "1800000000000"
		node.Params.MaxAgeNS = "1800000000000"
		if key == "window_stddev" {
			node.Params.MinSamples = "2"
		}
	}
	rule.Nodes = []Node{node}
	return rule, nil
}
