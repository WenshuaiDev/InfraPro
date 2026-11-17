package resources_test

import (
	"bytes"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/files"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"testing"
)

func TestStructureFilesFixedRevisionAndRevokedResourceDelivery(t *testing.T) {
	f := newFilesFixture(t)
	f.srv = f.b.Resources
	facility := f.typedFacility(t)
	hole := f.structureCreate(t, facility, res.Borehole, "FILE-HOLE", res.StructureRef{Kind: res.Facility, ID: facility.ID}, nil)
	project := f.project(t, "FileEvidenceProject")
	grants := f.accessInput(t, project, facility.ID, "create_link")
	grants.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}, {Kind: res.Borehole, BoreholeID: ptr(hole.ID), Capabilities: []string{"read", "update"}}}
	f.grant(t, project, facility.ID, grants)
	pf := f
	pf.actor = project.actor
	pf.scope = files.Scope{Kind: "project", TenantID: f.tenant, ProjectID: project.id}
	caps, e := pf.s.Capabilities(ctx, pf.actor, pf.scope)
	must(t, e)
	pf.epoch = caps.RestoreEpoch
	job := pf.run(t, pf.upload(t, []byte("original drilling evidence"), nil, nil))
	if job.State != "succeeded" {
		t.Fatal(job)
	}
	scope := res.ProjectScope(f.tenant, project.id)
	target := target(facility, res.Borehole, hole.ID)
	input := f.recordInput(t, facility, res.Borehole, hole.ID, "attachment")
	detail, e := f.srv.Detail(ctx, project.actor, scope, target)
	must(t, e)
	input.AccessRevision = detail.AccessRevision
	input.LinkRevision = detail.LinkRevision
	input.Attachment = &res.AttachmentRef{AttachmentID: valueID(job.ResultAttachmentID), RevisionID: valueID(job.ResultRevisionID)}
	input.Purpose = "Drilling report"
	preview, e := f.srv.PreviewStructureRecord(ctx, project.actor, scope, target, input)
	must(t, e)
	input.PreviewHash = preview.Hash
	_, e = f.srv.WriteStructureRecord(ctx, project.actor, scope, target, input, access.Meta{})
	must(t, e)
	records, e := f.srv.StructureRecords(ctx, project.actor, scope, target, "attachments", "", 25)
	must(t, e)
	if len(records.Items) != 1 {
		t.Fatal(records)
	}
	var item struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Revision string `json:"attachment_revision_id"`
	}
	must(t, json.Unmarshal(records.Items[0], &item))
	if item.Name == "" || item.Revision != input.Attachment.RevisionID {
		t.Fatal(item)
	}
	original, e := pf.s.Attachment(ctx, pf.actor, pf.scope, input.Attachment.AttachmentID)
	must(t, e)
	replacement := pf.run(t, pf.upload(t, []byte("replacement report"), &original.ID, &original.Revision))
	if replacement.State != "succeeded" {
		t.Fatal(replacement)
	}
	read, e := f.srv.OpenAttachment(ctx, project.actor, scope, target, item.ID, "source")
	must(t, e)
	var body bytes.Buffer
	must(t, read.Write(ctx, 0, read.Size, body.Write))
	must(t, read.Close())
	if body.String() != "original drilling evidence" {
		t.Fatal(body.String())
	}
	read, e = f.srv.OpenAttachment(ctx, project.actor, scope, target, item.ID, "source")
	must(t, e)
	accessDetail, e := f.srv.AccessDetail(ctx, f.actor, f.tenant, project.id, facility.ID, res.AccessFilter{})
	must(t, e)
	revoke := f.accessInput(t, project, facility.ID, "change_grants")
	for _, g := range accessDetail.Grants {
		if g.Kind == res.Borehole {
			revoke.End = append(revoke.End, res.GrantEnd{ID: g.ID, Revision: g.Revision})
		}
	}
	f.grant(t, project, facility.ID, revoke)
	sent := false
	e = read.Write(ctx, 0, read.Size, func(p []byte) (int, error) { sent = true; return len(p), nil })
	read.Close()
	if e == nil || sent {
		t.Fatal("revoked structure file escaped chunk guard")
	}
	var protections int
	must(t, f.env.Pool.QueryRow(ctx, `SELECT count(*) FROM file_protection_refs WHERE owner_module='engineering_structure' AND owner_id=$1 AND released_at IS NULL`, item.ID).Scan(&protections))
	if protections != 1 {
		t.Fatal("fixed engineering evidence not protected")
	}
}
