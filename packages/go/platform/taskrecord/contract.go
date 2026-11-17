// Package taskrecord owns metadata facts, not execution or authorization.
package taskrecord

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"slices"
	"strconv"
	"time"
)

var ErrConflict = errors.New("task_fact_conflict")

type SourceRef struct {
	Module string `json:"source_module"`
	Kind   string `json:"source_kind"`
	RootID string `json:"source_root_id"`
}

func (s SourceRef) Key() string { return s.Module + ":" + s.Kind + ":" + s.RootID }
func (s SourceRef) ID() string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("infrapro:task:"+s.Key())).String()
}

type Ref struct {
	SourceRef
	Generation string `json:"generation"`
	TaskID     string `json:"task_id,omitempty"`
}
type Scope struct {
	Kind    string `json:"kind"`
	Tenant  string `json:"tenant_id"`
	Project string `json:"project_id,omitempty"`
}
type Progress struct {
	Completed string  `json:"completed,omitempty"`
	Total     *string `json:"total"`
	Unit      string  `json:"unit,omitempty"`
}
type Change struct {
	SourceRef
	Type              string            `json:"type"`
	Scope             Scope             `json:"scope"`
	Originator        string            `json:"originator_account_id,omitempty"`
	OriginKind        string            `json:"origin_kind"`
	EventActor        string            `json:"event_actor,omitempty"`
	RequestedBy       string            `json:"requested_by,omitempty"`
	ExecutorKind      string            `json:"executor_kind"`
	ExecutorRef       string            `json:"executor_ref"`
	Generation        string            `json:"generation"`
	Revision          string            `json:"source_revision"`
	ControlRevision   string            `json:"control_revision"`
	State             string            `json:"state"`
	SourceState       string            `json:"source_state"`
	Phase             string            `json:"phase"`
	Progress          Progress          `json:"progress"`
	ErrorCode         string            `json:"error_code,omitempty"`
	InputRef          map[string]string `json:"input_ref"`
	InputHash         string            `json:"input_hash"`
	ResultRef         map[string]string `json:"result_ref"`
	CreatedAt         time.Time         `json:"created_at"`
	ObservedAt        time.Time         `json:"observed_at"`
	HistoryIncomplete bool              `json:"history_incomplete"`
	Fence             string            `json:"fence"`
	Attempt           int               `json:"attempt"`
	Event             string            `json:"event"`
	Reason            string            `json:"reason,omitempty"`
}
type Definition struct {
	Type        string   `json:"type"`
	Module      string   `json:"source_module"`
	Kind        string   `json:"source_kind"`
	Title       string   `json:"title"`
	Actions     []string `json:"actions"`
	Scopes      []string `json:"scopes"`
	Phases      []string `json:"phases"`
	Schema      int      `json:"schema_version"`
	DisplayOnly bool     `json:"display_only"`
}

var definitions = []Definition{
	{Type: "ingestion.reprocess", Module: "ingestion", Kind: "recovery", Title: "接入报文恢复", Actions: []string{"cancel", "retry"}, Scopes: []string{"platform", "tenant"}},
	{Type: "parsing.test", Module: "parsing", Kind: "run", Title: "样本试解析", Actions: []string{"cancel", "retry"}, Scopes: []string{"platform", "tenant"}},
	{Type: "audit.export", Module: "audit", Kind: "export", Title: "审计导出", Actions: []string{"cancel", "retry"}, Scopes: []string{"platform", "tenant", "project"}},
	{Type: "files.prepare", Module: "files", Kind: "job", Title: "文件检查与附件绑定", Actions: []string{"cancel", "retry"}},
	{Type: "monitoring.import.validate", Module: "monitoring", Kind: "job", Title: "导入校验", Actions: []string{"cancel"}},
	{Type: "monitoring.import.adopt", Module: "monitoring", Kind: "job", Title: "导入采用", Actions: []string{"cancel"}},
	{Type: "monitoring.export", Module: "monitoring", Kind: "job", Title: "监测数据导出", Actions: []string{"cancel"}},
	{Type: "monitoring.query", Module: "monitoring", Kind: "job", Title: "后台精确查询", Actions: []string{"cancel"}},
	{Type: "derived.trial", Module: "derived", Kind: "run", Title: "派生试算", Actions: []string{"cancel", "retry"}},
	{Type: "derived.backfill", Module: "derived", Kind: "run", Title: "历史回算", Actions: []string{"cancel", "retry"}},
	{Type: "derived.replay", Module: "derived", Kind: "run", Title: "原依据回放", Actions: []string{"cancel", "retry"}},
	{Type: "analysis.trial", Module: "derived", Kind: "run", Title: "历史分析试算", Actions: []string{"cancel", "retry"}},
	{Type: "analysis.run", Module: "derived", Kind: "run", Title: "历史分析", Actions: []string{"cancel", "retry"}},
	{Type: "analysis.compare", Module: "derived", Kind: "run", Title: "成果比较", Actions: []string{"cancel", "retry"}},
	{Type: "analysis.export", Module: "derived", Kind: "export", Title: "分析成果导出", Actions: []string{"cancel", "retry"}},
	{Type: "derived.evidence.export", Module: "derived", Kind: "export", Title: "计算依据导出", Actions: []string{"cancel", "retry"}},
	{Type: "spatial.process", Module: "spatial", Kind: "job", Title: "空间资料处理", Actions: []string{"cancel", "retry"}, Scopes: []string{"tenant"}},
	{Type: "display.package", Module: "display", Kind: "job", Title: "大屏包校验", Actions: []string{"cancel", "retry"}, Scopes: []string{"project"}},
	{Type: "derived.backfill_group", Module: "derived", Kind: "group", Title: "历史回算批次", Actions: []string{}, DisplayOnly: true},
}

func Catalog() []Definition {
	out := slices.Clone(definitions)
	for i := range out {
		out[i].Schema = 1
		out[i].Actions = slices.Clone(out[i].Actions)
		if len(out[i].Scopes) == 0 {
			out[i].Scopes = []string{"tenant", "project"}
		} else {
			out[i].Scopes = slices.Clone(out[i].Scopes)
		}
		out[i].Phases = []string{"queued", "preparing", "validating", "computing", "waiting", "writing", "sealing", "completed", "failed", "cancelled", "unknown"}
	}
	return out
}
func Lookup(kind string) (Definition, bool) {
	for _, d := range Catalog() {
		if d.Type == kind {
			return d, true
		}
	}
	return Definition{}, false
}
func Number(s string, minimum int64) (int64, error) {
	n, e := strconv.ParseInt(s, 10, 64)
	if e != nil || n < minimum || strconv.FormatInt(n, 10) != s {
		return 0, fmt.Errorf("invalid decimal integer")
	}
	return n, nil
}
func Terminal(state string) bool {
	return slices.Contains([]string{"succeeded", "failed", "cancelled", "needs_action"}, state)
}
func Active(state string) bool {
	return slices.Contains([]string{"queued", "running", "waiting", "retry_wait", "cancelling"}, state)
}
func Hash(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func JSON(v any) []byte { b, _ := json.Marshal(v); return b }
func Null(s string) any {
	if s == "" {
		return nil
	}
	return s
}
func (c Change) Validate() error {
	d, ok := Lookup(c.Type)
	if !ok || d.Module != c.Module || d.Kind != c.Kind {
		return fmt.Errorf("task_type_unavailable")
	}
	ids := []string{c.RootID}
	if c.Scope.Kind == "platform" {
		if (c.Type != "audit.export" && c.Type != "parsing.test" && c.Type != "ingestion.reprocess") || c.Scope.Tenant != "" || c.Scope.Project != "" {
			return fmt.Errorf("invalid platform task scope")
		}
	} else {
		ids = append(ids, c.Scope.Tenant)
	}
	for _, id := range ids {
		if _, e := uuid.Parse(id); e != nil {
			return fmt.Errorf("invalid task identity")
		}
	}
	if !slices.Contains(d.Scopes, c.Scope.Kind) || (c.Scope.Kind == "project") != (c.Scope.Project != "") {
		return fmt.Errorf("invalid task scope")
	}
	for _, id := range []string{c.Scope.Project, c.Originator, c.RequestedBy, c.EventActor} {
		if id != "" {
			if _, e := uuid.Parse(id); e != nil {
				return fmt.Errorf("invalid task account or project")
			}
		}
	}
	if c.OriginKind == "person" && c.Originator == "" {
		return fmt.Errorf("missing task originator")
	}
	if !slices.Contains([]string{"person", "delegated", "internal", "read_subject", "legacy_unknown"}, c.OriginKind) {
		return fmt.Errorf("invalid task origin")
	}
	for _, pair := range []struct {
		s   string
		min int64
	}{{c.Generation, 1}, {c.Revision, 0}, {c.ControlRevision, 0}, {c.Fence, 0}} {
		if _, e := Number(pair.s, pair.min); e != nil {
			return e
		}
	}
	if !Active(c.State) && !Terminal(c.State) && c.State != "unknown" {
		return fmt.Errorf("invalid task state")
	}
	if !slices.Contains(d.Phases, c.Phase) {
		return fmt.Errorf("invalid task phase")
	}
	if c.Progress.Completed != "" {
		if _, e := Number(c.Progress.Completed, 0); e != nil {
			return e
		}
		if !slices.Contains([]string{"rows", "bytes", "files", "units"}, c.Progress.Unit) {
			return fmt.Errorf("invalid task progress unit")
		}
	}
	if c.Progress.Total != nil {
		n, e := Number(*c.Progress.Total, 0)
		if e != nil {
			return e
		}
		completed, e := Number(c.Progress.Completed, 0)
		if e != nil || completed > n {
			return fmt.Errorf("invalid task progress total")
		}
	}
	for _, refs := range []map[string]string{c.InputRef, c.ResultRef} {
		for key, value := range refs {
			if key == "generation" {
				if _, e := Number(value, 1); e != nil {
					return e
				}
				continue
			}
			if !slices.Contains([]string{"job_id", "target_id", "version_id", "source_blob_id", "upload_id", "instance_id", "request_id", "prepared_file_id", "file_id", "attachment_id", "attachment_revision_id", "export_id", "object_id"}, key) {
				return fmt.Errorf("unregistered task reference")
			}
			if _, e := uuid.Parse(value); e != nil {
				return fmt.Errorf("task references require immutable IDs")
			}
		}
	}
	if c.InputHash == "" || len(c.InputHash) > 128 || len(c.ErrorCode) > 100 {
		return fmt.Errorf("invalid task digest or reason code")
	}
	if c.CreatedAt.IsZero() || c.ObservedAt.IsZero() || c.Attempt < 0 || len(JSON(c)) > 32768 || len(c.Reason) > 2000 {
		return fmt.Errorf("invalid task snapshot")
	}
	return nil
}
