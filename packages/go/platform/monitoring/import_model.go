package monitoring

import (
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"strings"
	"time"
)

const UploadPartSize int64 = 32 * 1024 * 1024
const MaxCSVBytes int64 = 32 * 1024 * 1024 * 1024
const MaxXLSXBytes int64 = 512 * 1024 * 1024
const MaxTemporaryBytes int64 = 8 * 1024 * 1024 * 1024

type ImportInput struct {
	WriteMeta
	Name string `json:"name"`
	Size string `json:"size_bytes"`
}
type UploadComplete struct {
	WriteMeta
	SHA256 string `json:"sha256"`
}
type ChannelMapping struct {
	Alias       string `json:"alias"`
	Channel     string `json:"channel_id"`
	ValueColumn string `json:"value_column"`
	Unit        string `json:"unit"`
}
type Mapping struct {
	Format          string            `json:"format"`
	Mode            string            `json:"mode"`
	Encoding        string            `json:"encoding"`
	Delimiter       string            `json:"delimiter"`
	Sheet           string            `json:"sheet"`
	HeaderRow       int               `json:"header_row"`
	ChannelColumn   string            `json:"channel_column"`
	TimeColumn      string            `json:"time_column"`
	ValueColumn     string            `json:"value_column"`
	UnitColumn      string            `json:"unit_column"`
	QualityColumn   string            `json:"quality_column"`
	RecordColumn    string            `json:"record_column"`
	IntervalColumn  string            `json:"interval_start_column"`
	Timezone        string            `json:"timezone"`
	LocalTimeLayout string            `json:"local_time_layout"`
	EmptyWide       string            `json:"empty_wide"`
	QualityMap      map[string]string `json:"quality_map"`
	Channels        []ChannelMapping  `json:"channels"`
}
type MappingInput struct {
	WriteMeta
	Mapping Mapping `json:"mapping"`
}
type ResolveInput struct {
	WriteMeta
	PreviewHash            string `json:"preview_hash"`
	ConfirmSameObservation bool   `json:"confirm_same_observation"`
}
type ImportCounts struct {
	Rows         string `json:"parsed_rows"`
	Candidates   string `json:"candidates"`
	BlankRows    string `json:"blank_rows"`
	MissingCells string `json:"missing_cells"`
	Errors       string `json:"errors"`
	Suspect      string `json:"suspect"`
	Duplicates   string `json:"duplicates"`
	Ignored      string `json:"ignored"`
	Proposed     string `json:"proposed"`
}
type ImportBatch struct {
	TaskRef         *taskrecord.Ref `json:"task_ref,omitempty"`
	ID              string          `json:"id"`
	State           string          `json:"state"`
	Revision        string          `json:"revision"`
	MappingRevision string          `json:"mapping_revision"`
	Name            string          `json:"name"`
	Size            string          `json:"size_bytes,omitempty"`
	SourceFile      string          `json:"source_file_id,omitempty"`
	Creator         string          `json:"created_by,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
	Counts          *ImportCounts   `json:"counts,omitempty"`
	ValidationHash  string          `json:"validation_hash,omitempty"`
	ErrorCode       string          `json:"error_code"`
	ReadoptBatch    string          `json:"readopt_batch_id,omitempty"`
	Parts           []UploadPart    `json:"parts"`
	Mapping         *Mapping        `json:"mapping,omitempty"`
	JobID           string          `json:"job_id,omitempty"`
	ScopeClipped    bool            `json:"scope_clipped"`
	VisibleAdopted  string          `json:"visible_adopted_count,omitempty"`
	VisibleCoverage []BatchCoverage `json:"visible_coverage,omitempty"`
	Preview         *SourcePreview  `json:"preview,omitempty"`
}
type UploadPart struct {
	Part   int    `json:"part"`
	Size   string `json:"size_bytes"`
	SHA256 string `json:"sha256"`
}
type ImportPage struct {
	Items      []ImportBatch `json:"items"`
	NextCursor string        `json:"next_cursor"`
}
type ImportIssue struct {
	Code        string `json:"code"`
	Line        string `json:"line"`
	Column      string `json:"column"`
	Channel     string `json:"channel_id"`
	At          string `json:"observed_at"`
	Message     string `json:"message"`
	Target      string `json:"target_observation_id,omitempty"`
	PayloadHash string `json:"payload_hash,omitempty"`
}
type IssuePage struct {
	Items       []ImportIssue `json:"items"`
	NextCursor  string        `json:"next_cursor"`
	PreviewHash string        `json:"preview_hash"`
}
type channelSemantics struct {
	ID          string          `json:"id"`
	Metric      string          `json:"metric_version_id"`
	Unit        string          `json:"unit_version_id"`
	Symbol      string          `json:"unit"`
	Kind        string          `json:"kind"`
	TimeMeaning string          `json:"time_meaning"`
	Direction   string          `json:"direction"`
	Basis       string          `json:"reference_basis"`
	Definition  json.RawMessage `json:"definition"`
	Hash        string          `json:"hash"`
}

func (m Mapping) validate() error {
	if m.Format != "csv" && m.Format != "xlsx" {
		return invalid("mapping.format", "请选择 CSV 或 XLSX。")
	}
	if m.Mode != "long" && m.Mode != "wide" && m.Mode != "single" {
		return invalid("mapping.mode", "请选择长表、宽表或单通道。")
	}
	if m.HeaderRow < 1 || m.HeaderRow > 10000 || m.TimeColumn == "" || len(m.Channels) == 0 || len(m.Channels) > 50000 {
		return invalid("mapping", "需要表头、时间列和明确的通道映射。")
	}
	if m.Mode == "long" && m.ChannelColumn == "" || m.Mode != "wide" && m.ValueColumn == "" || m.Mode == "single" && len(m.Channels) != 1 {
		return invalid("mapping", "列映射不完整。")
	}
	if m.Mode != "long" && m.ChannelColumn != "" || m.Mode == "wide" && m.ValueColumn != "" {
		return invalid("mapping", "未使用的通道／值列应留空，避免错误声明原文件覆盖范围。")
	}
	if m.Format == "csv" && ((m.Encoding != "utf-8" && m.Encoding != "gb18030") || !strings.Contains(",;\t|", m.Delimiter) || len(m.Delimiter) != 1) {
		return invalid("mapping", "CSV 编码或分隔符不受支持。")
	}
	if m.Format == "xlsx" && m.Sheet == "" {
		return invalid("mapping.sheet", "请选择工作表。")
	}
	if m.EmptyWide != "error" && m.EmptyWide != "no_observation" {
		return invalid("mapping.empty_wide", "请明确宽表空值处理方式。")
	}
	if _, e := time.LoadLocation(m.Timezone); e != nil {
		return invalid("mapping.timezone", "需要有效 IANA 时区。")
	}
	if len(m.LocalTimeLayout) > 128 || strings.Contains(m.LocalTimeLayout, "Z") || strings.Contains(m.LocalTimeLayout, "-07") || strings.Contains(m.LocalTimeLayout, "MST") {
		return invalid("mapping.local_time_layout", "本地时间格式不能含时区；带偏移时刻直接使用 RFC3339。")
	}
	aliases, columns := map[string]bool{}, map[string]bool{}
	for _, c := range m.Channels {
		if !access.ValidID(c.Channel) || c.Alias == "" || len(c.Alias) > 200 || aliases[c.Alias] {
			return invalid("mapping.channels", "通道标识或别名无效／重复。")
		}
		aliases[c.Alias] = true
		if m.Mode != "wide" && c.ValueColumn != "" {
			return invalid("mapping.channels", "只有宽表绑定单独的值列。")
		}
		if m.Mode == "wide" {
			if c.ValueColumn == "" || columns[c.ValueColumn] {
				return invalid("mapping.channels", "每个宽表值列必须明确且唯一。")
			}
			columns[c.ValueColumn] = true
		}
	}
	for _, v := range m.QualityMap {
		if v != "good" && v != "unmarked" && v != "suspect" && v != "bad" {
			return invalid("mapping.quality_map", "质量字典只能映射到四种标准质量。")
		}
	}
	return nil
}
