package config

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Deployment budgets never change numeric semantics or fixed rule defaults.
type Derived struct {
	ForegroundWorkers, HistoricalWorkers                           int
	RuleInputs, RuleNodes, RuleParameters, GraphDepth              int
	TrialRows, TrialExamples, WindowSamples, OutputRows, WriteRows int
	ScheduleBatch, TenantBatch, EventBatch                         int
	MemoryBytes, TemporaryBytes, ReadBytes, WriteBytes             uint64
	TrialRange, TrialDeadline, TrialTTL, ExportTTL, TemporaryTTL   time.Duration
}
type derivedInput struct {
	ForegroundWorkers string `yaml:"foreground_workers"`
	HistoricalWorkers string `yaml:"historical_workers"`
	RuleInputs        string `yaml:"rule_inputs"`
	RuleNodes         string `yaml:"rule_nodes"`
	RuleParameters    string `yaml:"rule_parameters"`
	GraphDepth        string `yaml:"graph_depth"`
	TrialRows         string `yaml:"trial_rows"`
	TrialExamples     string `yaml:"trial_examples"`
	WindowSamples     string `yaml:"window_samples"`
	OutputRows        string `yaml:"output_rows"`
	WriteRows         string `yaml:"write_rows"`
	ScheduleBatch     string `yaml:"schedule_batch"`
	TenantBatch       string `yaml:"tenant_batch"`
	EventBatch        string `yaml:"event_batch"`
	MemoryBytes       string `yaml:"memory_bytes"`
	TemporaryBytes    string `yaml:"temporary_bytes"`
	ReadBytes         string `yaml:"read_bytes"`
	WriteBytes        string `yaml:"write_bytes"`
	TrialRange        string `yaml:"trial_range"`
	TrialDeadline     string `yaml:"trial_deadline"`
	TrialTTL          string `yaml:"trial_ttl"`
	ExportTTL         string `yaml:"export_ttl"`
	TemporaryTTL      string `yaml:"temporary_ttl"`
}

func defaultDerivedInput() derivedInput {
	return derivedInput{"1", "1", "16", "32", "32", "8", "100000", "2000", "1000000", "1000000", "100000", "100", "16", "100", "1073741824", "21474836480", "268435456", "67108864", "24h", "3s", "30m", "168h", "24h"}
}
func derivedEnvironment(raw *derivedInput) {
	v := reflect.ValueOf(raw).Elem()
	typ := v.Type()
	for i := 0; i < v.NumField(); i++ {
		if value, ok := os.LookupEnv("INFRAPRO_DERIVED_" + strings.ToUpper(typ.Field(i).Tag.Get("yaml"))); ok {
			v.Field(i).SetString(value)
		}
	}
}
func DefaultDerived() Derived {
	v, e := resolveDerived(defaultDerivedInput())
	if e != nil {
		panic(e)
	}
	return v
}
func resolveDerived(raw derivedInput) (Derived, error) {
	out := Derived{}
	for _, f := range []struct {
		name, value string
		target      *int
		min, max    int
	}{
		{"foreground_workers", raw.ForegroundWorkers, &out.ForegroundWorkers, 1, 16}, {"historical_workers", raw.HistoricalWorkers, &out.HistoricalWorkers, 1, 16},
		{"rule_inputs", raw.RuleInputs, &out.RuleInputs, 1, 16}, {"rule_nodes", raw.RuleNodes, &out.RuleNodes, 1, 32}, {"rule_parameters", raw.RuleParameters, &out.RuleParameters, 1, 32}, {"graph_depth", raw.GraphDepth, &out.GraphDepth, 1, 8},
		{"trial_rows", raw.TrialRows, &out.TrialRows, 1, 100000}, {"trial_examples", raw.TrialExamples, &out.TrialExamples, 1, 2000}, {"window_samples", raw.WindowSamples, &out.WindowSamples, 1, 1000000}, {"output_rows", raw.OutputRows, &out.OutputRows, 1, 1000000}, {"write_rows", raw.WriteRows, &out.WriteRows, 1, 1000000},
		{"schedule_batch", raw.ScheduleBatch, &out.ScheduleBatch, 1, 100}, {"tenant_batch", raw.TenantBatch, &out.TenantBatch, 1, 100}, {"event_batch", raw.EventBatch, &out.EventBatch, 1, 1000},
	} {
		n, e := strconv.Atoi(f.value)
		if e != nil || n < f.min || n > f.max {
			return out, fmt.Errorf("derived.%s must be %d-%d", f.name, f.min, f.max)
		}
		*f.target = n
	}
	for _, f := range []struct {
		name, value string
		target      *uint64
		min, max    uint64
	}{
		{"memory_bytes", raw.MemoryBytes, &out.MemoryBytes, 64 << 20, 16 << 30}, {"temporary_bytes", raw.TemporaryBytes, &out.TemporaryBytes, 128 << 20, 1 << 40}, {"read_bytes", raw.ReadBytes, &out.ReadBytes, 1 << 20, 4 << 30}, {"write_bytes", raw.WriteBytes, &out.WriteBytes, 1 << 20, 128 << 20},
	} {
		n, e := strconv.ParseUint(f.value, 10, 64)
		if e != nil || n < f.min || n > f.max {
			return out, fmt.Errorf("derived.%s is outside the supported deployment budget", f.name)
		}
		*f.target = n
	}
	for _, f := range []struct {
		name, value string
		target      *time.Duration
		min, max    time.Duration
	}{
		{"trial_range", raw.TrialRange, &out.TrialRange, time.Second, 24 * time.Hour}, {"trial_deadline", raw.TrialDeadline, &out.TrialDeadline, 100 * time.Millisecond, 3 * time.Second}, {"trial_ttl", raw.TrialTTL, &out.TrialTTL, time.Minute, 24 * time.Hour}, {"export_ttl", raw.ExportTTL, &out.ExportTTL, time.Hour, 30 * 24 * time.Hour},
		{"temporary_ttl", raw.TemporaryTTL, &out.TemporaryTTL, time.Hour, 30 * 24 * time.Hour},
	} {
		n, e := time.ParseDuration(f.value)
		if e != nil || n < f.min || n > f.max {
			return out, fmt.Errorf("derived.%s is outside the supported deployment duration", f.name)
		}
		*f.target = n
	}
	if out.ReadBytes > out.MemoryBytes || out.WriteBytes > out.MemoryBytes/2 || out.WriteBytes > out.TemporaryBytes {
		return out, fmt.Errorf("derived read/write budgets exceed task memory or temporary storage")
	}
	return out, nil
}
func LoadDerived(path string) (Derived, error) {
	raw, e := loadInput(path)
	if e != nil {
		return Derived{}, e
	}
	return resolveDerived(raw.Derived)
}
