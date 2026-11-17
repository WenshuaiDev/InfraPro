//go:build monitoringintegration

package resources_test

import (
	"bytes"
	"fmt"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"testing"
)

func TestUploadLimitsMonitoringBeyondOldPartCount(t *testing.T) {
	f := monitoringSetup(t)
	f.service.WithUploadLimits(md.MaxCSVBytes+1, md.MaxXLSXBytes+1)
	in := md.ImportInput{WriteMeta: f.meta("0"), Name: "large.csv", Size: fmt.Sprint(md.MaxCSVBytes + 1)}
	batch, e := f.service.CreateImport(ctx, f.actor, f.scope, in)
	must(t, e)
	xlsx, e := f.service.CreateImport(ctx, f.actor, f.scope, md.ImportInput{WriteMeta: f.meta("0"), Name: "large.xlsx", Size: fmt.Sprint(md.MaxXLSXBytes + 1)})
	must(t, e)
	f.service.WithUploadLimits(1, 1)
	replayed, e := f.service.CreateImport(ctx, f.actor, f.scope, in)
	must(t, e)
	if replayed.ID != batch.ID {
		t.Fatal("quota reduction changed the accepted import receipt")
	}
	_, e = f.service.UploadPart(ctx, f.actor, f.scope, xlsx.ID, xlsx.Revision, 17, fileDigest([]byte("x")), bytes.NewReader([]byte("x")))
	must(t, e)
	_, e = f.service.UploadPart(ctx, f.actor, f.scope, batch.ID, batch.Revision, 1025, fileDigest([]byte("x")), bytes.NewReader([]byte("x")))
	must(t, e)
	_, e = f.service.UploadPart(ctx, f.actor, f.scope, batch.ID, batch.Revision, 1026, fileDigest([]byte("x")), bytes.NewReader([]byte("x")))
	if e == nil {
		t.Fatal("out-of-range monitoring part accepted")
	}
	for _, name := range []string{"small.csv", "small.xlsx"} {
		_, e = f.service.CreateImport(ctx, f.actor, f.scope, md.ImportInput{WriteMeta: f.meta("0"), Name: name, Size: "2"})
		if e == nil {
			t.Fatal("new upload ignored reduced quota", name)
		}
	}
}
