//go:build monitoringintegration

package resources_test

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
)

type exportPreflightDeadlineReader struct {
	md.CalculationReader
	first bool
}

func (r *exportPreflightDeadlineReader) CheckCalculated(ctx context.Context, a access.Actor, scope md.Scope, q md.CalculationQuery) ([]md.CalculationCoverage, error) {
	if r.first {
		r.first = false
		<-ctx.Done()
		// Policy adapters may intentionally hide the underlying DB timeout.
		return nil, access.ErrCatalog
	}
	return r.CalculationReader.CheckCalculated(ctx, a, scope, q)
}

type revokeAfterBlockWriter struct {
	bytes, writes int
	revoke        func() error
}

func (w *revokeAfterBlockWriter) Write(p []byte) (int, error) {
	w.bytes += len(p)
	w.writes++
	if w.writes == 1 {
		if e := w.revoke(); e != nil {
			return 0, e
		}
	}
	return len(p), nil
}

func derivedDownloadFixture(t *testing.T) (monitoringFixture, *dm.Service, md.ImportBatch, md.QueryInput) {
	t.Helper()
	f := monitoringSetup(t)
	s, channel, cfg := derivedChannelSetup(t, f)
	s.WithStorage(f.storage)
	must(t, dm.InitializeStorage(ctx, f.storage))
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	var content strings.Builder
	content.WriteString("observed_at,value,quality\n")
	for i := 0; i < 600; i++ {
		fmt.Fprintf(&content, "%s,%d,good\n", base.Add(time.Duration(i)*time.Second).Format(time.RFC3339Nano), i)
	}
	batch := f.publish(t, f.validate(t, f.upload(t, content.String())))
	scope := dm.TenantScope(f.tenant)
	from, to := strconv.FormatInt(base.UnixNano(), 10), strconv.FormatInt(base.Add(600*time.Second).UnixNano(), 10)
	trial, e := s.ChannelTrial(ctx, f.actor, scope, channel.Resource.ID, dm.TrialInput{WriteMeta: restoreMeta(channel.ConfigRevision), ConfigID: cfg.ID, ConfigHash: cfg.ConfigHash, OutputRange: dm.NanosecondRange{From: from, To: &to}}, access.Meta{})
	must(t, e)
	if trial.Job.State != "succeeded" {
		_, e = f.service.RunTask(ctx, "block-trial", f.tenant, trial.Job.ID)
		must(t, e)
		trial, e = s.Run(ctx, f.actor, scope, trial.ID)
		must(t, e)
	}
	preview, e := s.PreviewControl(ctx, f.actor, scope, channel.Resource.ID, dm.ControlPreviewInput{Revision: channel.ControlRevision, ControlProposal: dm.ControlProposal{Action: "enable", AutoStartNS: &from, StopNS: &to, TrialReceiptID: trial.Trial.ReceiptID}})
	must(t, e)
	_, e = s.ControlChannel(ctx, f.actor, scope, channel.Resource.ID, dm.ControlInput{WriteMeta: restoreMeta(channel.ControlRevision), PreviewID: preview.ID, PreviewHash: preview.Hash}, access.Meta{})
	must(t, e)
	_, e = s.ScheduleTenant(ctx, f.tenant)
	must(t, e)
	var job string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT job_id::text FROM derived_runs WHERE tenant_id=$1 AND channel_id=$2 AND mode='automatic'`, f.tenant, channel.Resource.ID).Scan(&job))
	_, e = f.service.RunTask(ctx, "block-formal", f.tenant, job)
	must(t, e)
	selection, e := f.service.Select(ctx, f.actor, f.scope, md.SelectionInput{Channels: []string{channel.Resource.ID}})
	must(t, e)
	lo, hi := base.Format(time.RFC3339Nano), base.Add(600*time.Second).Format(time.RFC3339Nano)
	query := md.QueryInput{Selection: selection.ID, Range: &md.Range{From: &lo, To: &hi}}
	return f, s, batch, query
}

func TestDerivedExportPreflightHandsOffAndRechecksRevocation(t *testing.T) {
	f, s, batch, query := derivedDownloadFixture(t)
	f.service.WithCalculationReader(&exportPreflightDeadlineReader{CalculationReader: s, first: true})
	created := time.Now()
	queued, e := f.service.CreateExport(ctx, f.actor, f.scope, md.ExportInput{WriteMeta: f.meta("0"), Query: query, Kind: "details", Format: "csv"})
	must(t, e)
	if time.Since(created) > time.Second || queued.State != "queued" {
		t.Fatal("slow preflight did not hand off within submission budget", queued.State, time.Since(created))
	}
	// Revocation after handoff must be discovered by the actual worker; a
	// timed-out preflight grants no authority and exposes no download files.
	p, e := f.service.PreviewRevokeBatch(ctx, f.actor, f.scope, batch.ID, batch.Revision)
	must(t, e)
	_, e = f.service.RevokeBatch(ctx, f.actor, f.scope, batch.ID, md.RevokeInput{WriteMeta: f.meta(batch.Revision), PreviewHash: p.Hash})
	must(t, e)
	_, e = f.service.RunTask(ctx, "preflight-revoked-export", f.tenant, queued.JobID)
	if e == nil {
		t.Fatal("background qualification accepted revoked derived input")
	}
	if _, e = f.service.ExportFiles(ctx, f.actor, f.scope, queued.ID); e == nil {
		t.Fatal("unqualified export exposed files")
	}
}

func TestDerivedCurrentExportStopsAfterRevokedDownloadBlock(t *testing.T) {
	f, _, batch, query := derivedDownloadFixture(t)
	manifest := f.completeExport(t, md.ExportInput{WriteMeta: f.meta("0"), Query: query, Kind: "details", Format: "csv"})
	download, e := f.service.ExportDownload(ctx, f.actor, f.scope, manifest.ID, "1")
	must(t, e)
	if download.Size <= 128*1024 {
		t.Fatal("fixture did not cross a download authorization block", download.Size)
	}
	writer := &revokeAfterBlockWriter{revoke: func() error {
		p, e := f.service.PreviewRevokeBatch(ctx, f.actor, f.scope, batch.ID, batch.Revision)
		if e != nil {
			return e
		}
		_, e = f.service.RevokeBatch(ctx, f.actor, f.scope, batch.ID, md.RevokeInput{WriteMeta: f.meta(batch.Revision), PreviewHash: p.Hash})
		return e
	}}
	if e = download.Write(ctx, writer); e == nil || writer.writes != 1 || writer.bytes != 128*1024 {
		t.Fatal("download sent a second block after source revocation", e, writer.writes, writer.bytes)
	}
}
