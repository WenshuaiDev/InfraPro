package monitoring

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strconv"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

// ManagedExportRecord shares monitoring_exports and monitoring_jobs. The source
// domain supplies immutable provenance and current authorization; it cannot
// publish a file independently of the host lease or expose an object URL.
type ManagedExportRecord struct {
	Content       string    `json:"content"`
	ID            string    `json:"id"`
	Job           Job       `json:"job"`
	SourceKind    string    `json:"source_kind"`
	SourceVersion string    `json:"source_version_id"`
	Format        string    `json:"format"`
	ExpiresAt     time.Time `json:"expires_at"`
	manifest      string
	invalidated   bool
}
type ResultExportSpec struct {
	Schema   string          `json:"schema"`
	Source   json.RawMessage `json:"source"`
	CSVMode  string          `json:"csv_mode"`
	Fields   []ExportField   `json:"fields"`
	Ordering string          `json:"ordering"`
}
type ResultExportManifest struct {
	ResultExportSpec
	ID             string       `json:"id"`
	SourceKind     string       `json:"source_kind"`
	SourceVersion  string       `json:"source_version_id"`
	Format         string       `json:"format"`
	Escaping       string       `json:"escaping"`
	ParquetVersion string       `json:"parquet_version"`
	Files          []ExportFile `json:"files"`
	Rows           string       `json:"row_count"`
	ExpiresAt      time.Time    `json:"expires_at"`
}
type sealedResultExport struct {
	Manifest ResultExportManifest `json:"manifest"`
	Objects  map[string]string    `json:"objects"`
}

func ValidateResultExport(format, mode string) error {
	if !slices.Contains([]string{"csv", "parquet"}, format) {
		return invalid("format", "请选择 CSV 或 Parquet。")
	}
	if !slices.Contains([]string{"exact", "spreadsheet_safe"}, mode) || format == "parquet" && mode != "exact" {
		return invalid("csv_mode", "CSV 模式无效。")
	}
	return nil
}
func (s *Service) WithResultExportTTL(ttl time.Duration) *Service {
	if ttl > 0 {
		s.resultExportTTL = ttl
	}
	return s
}

func (s *Service) CreateResultExportTx(ctx context.Context, tx pgx.Tx, in EnqueueTask, version, format string) (ManagedExportRecord, error) {
	var out ManagedExportRecord
	if in.Kind != "analysis_export" && in.Kind != "derived_evidence_export" || !access.ValidID(version) {
		return out, invalid("source_version_id", "需要固定成果版本。")
	}
	j, e := s.EnqueueTaskTx(ctx, tx, in)
	if e != nil {
		return out, e
	}
	column, kind := "artifact_version_id", "analysis"
	if in.Kind == "derived_evidence_export" {
		column, kind = "generation_id", "derived_evidence"
	}
	if _, e = tx.Exec(ctx, `INSERT INTO monitoring_exports(id,tenant_id,job_id,source_kind,`+column+`,format,expires_at) VALUES($1,$2,$3,$7,$4,$5,clock_timestamp()+($6::bigint * interval '1 millisecond'))`, j.Target, in.Scope.Tenant, j.ID, version, format, s.resultExportTTL.Milliseconds(), kind); e != nil {
		return out, e
	}
	return s.ResultExportTx(ctx, tx, in.Scope, j.Target, in.ActorID)
}

// ResultExportTx only loads an actor-owned descriptor. The caller must apply
// its domain policy in this same transaction before returning any metadata.
func (s *Service) ResultExportTx(ctx context.Context, tx pgx.Tx, scope Scope, id, actor string) (ManagedExportRecord, error) {
	var out ManagedExportRecord
	if !scope.valid() || !access.ValidID(id) || !access.ValidID(actor) {
		return out, notFound()
	}
	var job string
	e := tx.QueryRow(ctx, `SELECT e.id::text,e.job_id::text,e.source_kind,COALESCE(e.artifact_version_id,e.generation_id)::text,e.format,e.expires_at,COALESCE(e.manifest_object,''),e.invalidated_at IS NOT NULL,CASE WHEN e.source_kind='derived_evidence' THEN 'inputs' ELSE COALESCE(j.input->>'content','results') END FROM monitoring_exports e JOIN monitoring_jobs j ON j.tenant_id=e.tenant_id AND j.id=e.job_id WHERE e.tenant_id=$1 AND e.id=$2 AND e.source_kind IN ('analysis','derived_evidence') AND j.actor_id=$3 AND j.project_id IS NOT DISTINCT FROM $4::uuid`, scope.Tenant, id, actor, nullable(scope.Project)).Scan(&out.ID, &job, &out.SourceKind, &out.SourceVersion, &out.Format, &out.ExpiresAt, &out.manifest, &out.invalidated, &out.Content)
	if e == pgx.ErrNoRows {
		return out, notFound()
	}
	if e != nil {
		return out, e
	}
	out.Job, e = scanJob(tx.QueryRow(ctx, jobSelect+` WHERE tenant_id=$1 AND id=$2`, scope.Tenant, job))
	return out, e
}
func (r ManagedExportRecord) CheckAvailable() error {
	if r.invalidated {
		return problem("export_invalidated", "导出已失效，请重新申请。", 409)
	}
	if !r.ExpiresAt.After(time.Now()) {
		return problem("export_expired", "导出结果已过期，请重新申请。", 410)
	}
	return nil
}
func (s *Service) GenerateResultExport(ctx context.Context, lease TaskLease, spec ResultExportSpec, scan func(func([]ExportCell) error) error) error {
	if lease.service != s || scan == nil || len(spec.Fields) == 0 || len(spec.Fields) > 100 || len(spec.Source) > 1<<20 {
		return invalid("export", "导出定义不完整。")
	}
	var record ManagedExportRecord
	if e := lease.Transaction(ctx, func(tx pgx.Tx) error {
		var e error
		record, e = s.ResultExportTx(ctx, tx, lease.Task().Scope, lease.job.Target, lease.job.actor)
		if e != nil {
			return e
		}
		return record.CheckAvailable()
	}); e != nil {
		return e
	}
	if e := ValidateResultExport(record.Format, spec.CSVMode); e != nil {
		return e
	}
	guard := func() error {
		return lease.Transaction(ctx, func(tx pgx.Tx) error {
			r, e := s.ResultExportTx(ctx, tx, lease.Task().Scope, record.ID, lease.job.actor)
			if e != nil {
				return e
			}
			return r.CheckAvailable()
		})
	}
	volumes, e := s.exportVolumes(ctx, lease.job, record.Format, spec.CSVMode, record.SourceKind, spec.Fields, guard)
	if e != nil {
		return e
	}
	defer volumes.abort()
	if e = scan(volumes.write); e != nil {
		return e
	}
	if e = volumes.finish(); e != nil {
		return e
	}
	manifest := ResultExportManifest{ResultExportSpec: spec, ID: record.ID, SourceKind: record.SourceKind, SourceVersion: record.SourceVersion, Format: record.Format, Files: volumes.files, Rows: countText(volumes.total), ExpiresAt: record.ExpiresAt}
	if record.Format == "parquet" {
		manifest.ParquetVersion = "2.6; UTC NANOS; computed_decimal_text UTF-8 + scale/rounding (not DECIMAL)"
	}
	if spec.CSVMode == "spreadsheet_safe" {
		manifest.Escaping = "Non-numeric formula-like cells gain one leading apostrophe; remove exactly that prefix to reverse. Numeric values are unchanged."
	}
	key, e := s.storage.putSummary(ctx, lease.job.tenant, sealedResultExport{manifest, volumes.objects})
	if e != nil {
		return e
	}
	return lease.Transaction(ctx, func(tx pgx.Tx) error {
		tag, e := tx.Exec(ctx, `UPDATE monitoring_exports SET manifest_object=$3 WHERE tenant_id=$1 AND id=$2 AND invalidated_at IS NULL AND expires_at>clock_timestamp()`, lease.job.tenant, record.ID, key)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return problem("export_invalidated", "导出已失效。", 409)
		}
		_, e = tx.Exec(ctx, `UPDATE monitoring_jobs SET state='succeeded',progress=$2,error_code=NULL,lease_owner=NULL,lease_until=NULL,revision=revision+1,updated_at=clock_timestamp() WHERE id=$1`, lease.job.ID, encodeJSON(map[string]string{"phase": "complete", "rows": countText(volumes.total), "files": strconv.Itoa(volumes.part)}))
		return e
	})
}
func (s *Service) loadResultExport(ctx context.Context, r ManagedExportRecord, guard func(context.Context) error) (sealedResultExport, error) {
	var out sealedResultExport
	if guard == nil {
		return out, access.ErrDenied
	}
	if e := guard(ctx); e != nil {
		return out, e
	}
	if e := r.CheckAvailable(); e != nil {
		return out, e
	}
	if r.Job.State != "succeeded" || r.manifest == "" {
		return out, problem("export_not_ready", "导出尚未完成，请查看任务状态。", 409)
	}
	if e := s.storage.summaryObject(ctx, r.manifest, &out, true); e != nil {
		return out, storageError(e)
	}
	if out.Manifest.ID != r.ID || out.Manifest.SourceKind != r.SourceKind || out.Manifest.SourceVersion != r.SourceVersion || out.Manifest.Format != r.Format || !out.Manifest.ExpiresAt.Equal(r.ExpiresAt) || len(out.Manifest.Files) != len(out.Objects) {
		return out, problem("result_integrity_unavailable", "导出清单与固定版本不一致。", 503)
	}
	if e := guard(ctx); e != nil {
		return out, e
	}
	return out, nil
}
func (s *Service) ResultExportFiles(ctx context.Context, r ManagedExportRecord, guard func(context.Context) error) (ResultExportManifest, error) {
	out, e := s.loadResultExport(ctx, r, guard)
	return out.Manifest, e
}
func (s *Service) ResultExportDownload(ctx context.Context, r ManagedExportRecord, part string, guard func(context.Context) error) (Download, error) {
	result, e := s.loadResultExport(ctx, r, guard)
	if e != nil {
		return Download{}, e
	}
	out := Download{ContentType: "application/octet-stream"}
	var data []byte
	if part == "manifest" {
		data = encodeJSON(result.Manifest)
		out.Name = r.SourceKind + "-export-manifest.json"
		out.ContentType = "application/json"
		out.Size = int64(len(data))
		sum := sha256.Sum256(data)
		out.SHA256 = hex.EncodeToString(sum[:])
	} else {
		for _, f := range result.Manifest.Files {
			if f.Part == part {
				out.Name = f.Name
				out.Size, e = strconv.ParseInt(f.Bytes, 10, 64)
				if e != nil {
					return Download{}, e
				}
				out.SHA256 = f.SHA256
				break
			}
		}
		if out.Name == "" || result.Objects[part] == "" {
			return Download{}, notFound()
		}
	}
	out.Write = func(ctx context.Context, w io.Writer) error {
		if e := guard(ctx); e != nil {
			return e
		}
		var reader io.ReadCloser
		if part == "manifest" {
			reader = io.NopCloser(bytes.NewReader(data))
		} else {
			var e error
			reader, e = s.storage.get(ctx, result.Objects[part])
			if e != nil {
				return storageError(e)
			}
		}
		defer reader.Close()
		buffer := make([]byte, 128*1024)
		hash := sha256.New()
		var written int64
		for {
			n, readError := io.ReadFull(reader, buffer)
			if readError == io.ErrUnexpectedEOF {
				readError = io.EOF
			}
			if n > 0 {
				// Requalify after potentially slow external I/O and before emitting bytes.
				if e := guard(ctx); e != nil {
					return e
				}
				hash.Write(buffer[:n])
				count, e := w.Write(buffer[:n])
				written += int64(count)
				if e != nil {
					return e
				}
				if count != n {
					return io.ErrShortWrite
				}
			}
			if readError == io.EOF {
				if written != out.Size || hex.EncodeToString(hash.Sum(nil)) != out.SHA256 {
					return fmt.Errorf("export integrity mismatch")
				}
				return guard(ctx)
			}
			if readError != nil {
				return storageError(readError)
			}
		}
	}
	return out, nil
}
