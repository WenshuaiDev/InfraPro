package monitoring

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// ExportCell is lossless text plus explicit nullability. Domains provide rows;
// the shared exporter owns CSV/Parquet encoding, bounded files and verification.
type ExportCell = exportCell

type exportVolumes struct {
	service                       *Service
	ctx                           context.Context
	job                           Job
	format, mode, kind, directory string
	fields                        []ExportField
	guard                         func() error
	writer                        *exportWriter
	total                         int64
	part                          int
	files                         []ExportFile
	objects                       map[string]string
	lastCheck                     time.Time
}

func (s *Service) exportVolumes(ctx context.Context, j Job, format, mode, kind string, fields []ExportField, guard func() error) (*exportVolumes, error) {
	directory, e := os.MkdirTemp(s.storage.config.TempDirectory, "monitoring-export-")
	if e != nil {
		return nil, e
	}
	return &exportVolumes{service: s, ctx: ctx, job: j, format: format, mode: mode, kind: kind, fields: fields, guard: guard, directory: directory, files: []ExportFile{}, objects: map[string]string{}, lastCheck: time.Now()}, nil
}
func (v *exportVolumes) abort() {
	if v.writer != nil {
		v.writer.file.Close()
	}
	os.RemoveAll(v.directory)
}
func (v *exportVolumes) open() error {
	v.part++
	w, e := newExportWriter(filepath.Join(v.directory, strconv.Itoa(v.part)), v.format, v.mode, v.fields)
	v.writer = w
	return e
}
func (v *exportVolumes) write(cells []exportCell) error {
	if e := v.ctx.Err(); e != nil {
		return e
	}
	if time.Since(v.lastCheck) >= time.Second {
		if e := v.guard(); e != nil {
			return e
		}
		v.lastCheck = time.Now()
	}
	if v.writer == nil {
		if e := v.open(); e != nil {
			return e
		}
	}
	if e := v.writer.write(cells); e != nil {
		return e
	}
	v.total++
	size, e := v.writer.size()
	if e != nil {
		return e
	}
	if v.writer.rows >= v.service.storage.config.ExportRows || size >= v.service.storage.config.ExportBytes {
		return v.seal()
	}
	return nil
}
func (v *exportVolumes) seal() error {
	if v.writer == nil {
		return nil
	}
	if e := v.writer.close(); e != nil {
		return e
	}
	path, rows := v.writer.file.Name(), v.writer.rows
	v.writer = nil
	hash, size, e := verifyExportVolume(path, v.format, rows)
	if e != nil {
		return e
	}
	if e = v.guard(); e != nil {
		return e
	}
	name := fmt.Sprintf("monitoring-%s-%06d.%s", v.kind, v.part, v.format)
	key := fmt.Sprintf("%s/exports/%s/%d/%s", v.job.tenant, v.job.Target, v.job.fence, name)
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	e = v.service.storage.put(v.ctx, key, f, size, "application/octet-stream")
	f.Close()
	if e != nil {
		return e
	}
	file := ExportFile{strconv.Itoa(v.part), name, countText(rows), countText(size), hash}
	v.files = append(v.files, file)
	v.objects[file.Part] = key
	if e = os.Remove(path); e != nil {
		return e
	}
	return v.service.recordProgress(v.ctx, v.job, map[string]string{"phase": "writing", "rows": countText(v.total), "completed_files": strconv.Itoa(v.part)})
}
func (v *exportVolumes) finish() error {
	if v.writer == nil && len(v.files) == 0 {
		if e := v.open(); e != nil {
			return e
		}
	}
	return v.seal()
}
