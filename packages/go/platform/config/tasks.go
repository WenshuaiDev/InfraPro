package config

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

type Tasks struct {
	PageSize, MaxPageSize, CandidateBatch, CandidateBudget, HandoffBatch, RetryLimit, ReconcileBatch int
	MetadataTimeout, Poll, ReconcileInterval, ShutdownGrace                                          time.Duration
}
type tasksInput struct {
	PageSize          string `yaml:"list_page_size"`
	MaxPageSize       string `yaml:"max_page_size"`
	CandidateBatch    string `yaml:"candidate_batch"`
	CandidateBudget   string `yaml:"candidate_budget"`
	HandoffBatch      string `yaml:"handoff_batch"`
	RetryLimit        string `yaml:"handoff_retry_limit"`
	ReconcileBatch    string `yaml:"reconcile_batch"`
	MetadataTimeout   string `yaml:"metadata_timeout"`
	Poll              string `yaml:"poll_interval"`
	ReconcileInterval string `yaml:"reconcile_interval"`
	ShutdownGrace     string `yaml:"shutdown_grace"`
}

func defaultTasksInput() tasksInput {
	return tasksInput{"50", "100", "200", "2000", "100", "8", "500", "5s", "1s", "5m", "10s"}
}
func tasksEnvironment(raw *tasksInput) {
	v := reflect.ValueOf(raw).Elem()
	t := v.Type()
	for n := 0; n < v.NumField(); n++ {
		if value, ok := os.LookupEnv("INFRAPRO_TASKS_" + strings.ToUpper(t.Field(n).Tag.Get("yaml"))); ok {
			v.Field(n).SetString(value)
		}
	}
}
func parseTasks(raw tasksInput) (Tasks, error) {
	var out Tasks
	values := reflect.ValueOf(raw)
	fields := reflect.ValueOf(&out).Elem()
	for n := 0; n < 7; n++ {
		v, e := strconv.Atoi(values.Field(n).String())
		if e != nil || v < 1 || v > 10000 {
			return out, fmt.Errorf("invalid task integer setting")
		}
		fields.Field(n).SetInt(int64(v))
	}
	for n := 7; n < values.NumField(); n++ {
		v, e := time.ParseDuration(values.Field(n).String())
		if e != nil || v < time.Millisecond || v > time.Hour {
			return out, fmt.Errorf("invalid task duration setting")
		}
		fields.Field(n).SetInt(int64(v))
	}
	if out.MaxPageSize > 100 || out.PageSize > out.MaxPageSize || out.CandidateBatch > out.CandidateBudget || out.MetadataTimeout > 30*time.Second {
		return out, fmt.Errorf("task budgets out of bounds")
	}
	return out, nil
}
func LoadTasks(path string) (Database, Tasks, error) {
	db, e := LoadDatabase(path)
	if e != nil {
		return db, Tasks{}, e
	}
	raw, e := loadInput(path)
	if e != nil {
		return db, Tasks{}, e
	}
	o, e := parseTasks(raw.Tasks)
	return db, o, e
}
