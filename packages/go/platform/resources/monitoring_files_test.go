//go:build monitoringintegration

package resources_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/xuri/excelize/v2"
)

func TestMonitoringXLSXWideDateSystemsAndFormulaBlock(t *testing.T) {
	f := monitoringSetup(t)
	for _, date1904 := range []bool{false, true} {
		workbook := excelize.NewFile()
		must(t, workbook.SetWorkbookProps(&excelize.WorkbookPropsOptions{Date1904: &date1904}))
		must(t, workbook.SetCellStr("Sheet1", "A1", "observed_at"))
		must(t, workbook.SetCellStr("Sheet1", "B1", "displacement"))
		serial := 1
		if date1904 {
			serial = 0
		}
		must(t, workbook.SetCellInt("Sheet1", "A2", int64(serial)))
		must(t, workbook.SetCellStr("Sheet1", "B2", "10000000000000000000000000.000000000001"))
		raw, e := workbook.WriteToBuffer()
		must(t, e)
		mapping := md.Mapping{Format: "xlsx", Mode: "wide", Sheet: "Sheet1", HeaderRow: 1, TimeColumn: "observed_at", Timezone: "UTC", EmptyWide: "error", Channels: []md.ChannelMapping{{Alias: "C", Channel: f.channel, ValueColumn: "displacement", Unit: "mm"}}}
		upload := f.uploadBytes(t, "synthetic.xlsx", raw.Bytes(), mapping)
		preview, e := f.service.Import(ctx, f.actor, f.scope, upload.ID)
		must(t, e)
		if preview.Preview == nil || !preview.Preview.Available || len(preview.Preview.Sheets) != 1 || len(preview.Preview.Rows) != 2 || preview.Preview.Rows[1][1] != "10000000000000000000000000.000000000001" {
			t.Fatal("bounded XLSX preview", preview.Preview)
		}
		batch := f.validate(t, upload)
		if batch.State != "ready" || batch.Counts.Proposed != "1" {
			t.Fatal(batch)
		}
		f.publish(t, batch)
		must(t, workbook.SetCellFormula("Sheet1", "B2", "1+1"))
		formula, e := workbook.WriteToBuffer()
		must(t, e)
		blocked := f.validate(t, f.uploadBytes(t, "formula.xlsx", formula.Bytes(), mapping))
		if blocked.State != "needs_resolution" || blocked.Counts.Errors == "0" {
			t.Fatal("formula cache accepted", blocked)
		}
		must(t, workbook.Close())
	}
	query := f.query(t)
	page, e := f.service.Details(ctx, f.actor, f.scope, query)
	must(t, e)
	if len(page.Items) != 2 || page.Items[0].At != "1904-01-01T00:00:00Z" || page.Items[1].At != "1900-01-01T00:00:00Z" {
		t.Fatal(page)
	}
	for _, row := range page.Items {
		if row.Value != "10000000000000000000000000.000000000001" {
			t.Fatal(row)
		}
	}
}

func TestMonitoringMultipartResumeAndOriginalDownload(t *testing.T) {
	f := monitoringSetup(t)
	data := []byte(strings.Repeat("a,b,c\n", int(md.UploadPartSize)/6+32))
	size := len(data)
	batch, e := f.service.CreateImport(ctx, f.actor, f.scope, md.ImportInput{WriteMeta: f.meta("0"), Name: "multipart.csv", Size: fmt.Sprint(size)})
	must(t, e)
	completeHash := sha256.Sum256(data)
	complete := md.UploadComplete{WriteMeta: f.meta(batch.Revision), SHA256: hex.EncodeToString(completeHash[:])}
	tail := data[md.UploadPartSize:]
	tailHash := sha256.Sum256(tail)
	_, e = f.service.UploadPart(ctx, f.actor, f.scope, batch.ID, batch.Revision, 2, hex.EncodeToString(tailHash[:]), bytes.NewReader(tail))
	must(t, e)
	resumed, e := f.service.Import(ctx, f.actor, f.scope, batch.ID)
	must(t, e)
	if len(resumed.Parts) != 1 || resumed.Parts[0].Part != 2 {
		t.Fatal(resumed)
	}
	_, e = f.service.CompleteUpload(ctx, f.actor, f.scope, batch.ID, complete)
	monitoringCode(t, e, "monitoring_invalid")
	headHash := sha256.Sum256(data[:md.UploadPartSize])
	_, e = f.service.UploadPart(ctx, f.actor, f.scope, batch.ID, batch.Revision, 1, hex.EncodeToString(headHash[:]), bytes.NewReader(data[:md.UploadPartSize]))
	must(t, e)
	batch, e = f.service.CompleteUpload(ctx, f.actor, f.scope, batch.ID, complete)
	must(t, e)
	replay, e := f.service.CompleteUpload(ctx, f.actor, f.scope, batch.ID, complete)
	must(t, e)
	if replay.SourceFile != batch.SourceFile {
		t.Fatal("completion receipt changed")
	}
	download, e := f.service.Source(ctx, f.actor, f.scope, batch.SourceFile)
	must(t, e)
	hash := sha256.New()
	must(t, download.Write(ctx, hash))
	if download.Size != int64(size) || hex.EncodeToString(hash.Sum(nil)) != complete.SHA256 {
		t.Fatal("download changed complete source bytes")
	}
}
