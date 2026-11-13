//go:build monitoringintegration

package resources_test

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	md "github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	res "github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
	"github.com/jackc/pgx/v5"
)

func TestAnalysisSelectionRequiresOnlyCompleteSelectedDependencies(t *testing.T) {
	f := monitoringSetup(t)
	s, _, cfg := derivedChannelSetup(t, f)
	s.WithStorage(f.storage)
	must(t, dm.InitializeStorage(ctx, f.storage))
	f.publish(t, f.validate(t, f.upload(t, "observed_at,value,quality\n2024-01-01T00:00:00Z,2,good\n2024-01-01T00:01:00Z,5,good\n2024-01-01T00:02:00Z,9,good\n")))
	project := f.project(t, "SelectedAnalysis")
	scope := dm.ProjectScope(f.tenant, project.id)
	var facility, resourceGrant string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT facility_id::text FROM engineering_channels WHERE id=$1`, f.channel).Scan(&facility))
	grant := f.accessInput(t, project, facility, "create_link")
	grant.Add = []res.GrantInput{{Kind: res.Facility, Capabilities: []string{"read"}}, {Kind: res.Channel, ChannelID: &f.channel, Capabilities: []string{"read"}}}
	f.grant(t, project, facility, grant)
	must(t, f.env.Pool.QueryRow(ctx, `SELECT id::text FROM project_resource_grants WHERE project_id=$1 AND channel_id=$2 AND ended_at IS NULL`, project.id, f.channel).Scan(&resourceGrant))
	fromText, toText := "2024-01-01T00:00:00Z", "2024-01-01T00:03:00Z"
	read, e := f.service.GrantData(ctx, f.actor, f.tenant, project.id, md.GrantInput{WriteMeta: f.meta("0"), ChannelID: f.channel, ResourceGrantID: resourceGrant, Range: &md.Range{To: &toText}, Capabilities: []string{"read"}})
	must(t, e)
	var rule dm.Rule
	must(t, json.Unmarshal(cfg.Rule, &rule))
	difference := rule.Nodes[0]
	difference.ID = "delta"
	rule.Nodes = []dm.Node{{ID: "point", Op: "formula", Params: dm.NodeParams{Expression: "x", Variables: map[string]string{"x": "input:x"}}}, difference}
	point := rule.Outputs[0]
	point.Key, point.Node, point.SemanticKey, point.Quantity = "point", "point", "selected_point", nil
	delta := point
	delta.Key, delta.Node, delta.SemanticKey = "delta", "delta", "selected_delta"
	rule.Outputs = []dm.Output{point, delta}
	raw, e := json.Marshal(rule)
	must(t, e)
	created, e := s.CreateAnalysis(ctx, project.actor, scope, dm.CreateAnalysisInput{WriteMeta: restoreMeta(""), Code: "SELECTED", Name: "局部成果范围", Body: dm.AnalysisBody{Rule: raw, InputBindings: cfg.InputBindings, Parameters: cfg.Parameters}}, access.Meta{})
	must(t, e)
	validation, e := s.ValidateAnalysis(ctx, project.actor, scope, created.Config.ID, dm.ValidateInput{DraftRevision: created.Config.Draft.Revision})
	must(t, e)
	published, e := s.PublishAnalysis(ctx, project.actor, scope, created.Config.ID, dm.PublishAnalysisInput{PublishMetricInput: dm.PublishMetricInput{WriteMeta: restoreMeta(created.Config.Revision), DraftRevision: created.Config.Draft.Revision, ValidationHash: validation.ValidationHash}}, access.Meta{})
	must(t, e)
	from, to := "1704067200000000000", "1704067380000000000"
	run, e := s.StartAnalysis(ctx, project.actor, scope, dm.AnalysisRunInput{WriteMeta: restoreMeta(""), ConfigVersionID: published.Version.ID, ConfigHash: published.Version.ConfigHash, OutputRange: dm.NanosecondRange{From: from, To: &to}}, false, access.Meta{})
	must(t, e)
	_, e = f.service.RunTask(ctx, "complete-analysis-before-restriction", f.tenant, run.Job.ID)
	must(t, e)
	artifact, version := run.Analysis.ArtifactID, run.Analysis.ArtifactVersionID
	preview, e := f.service.PreviewRevokeData(ctx, f.actor, f.tenant, project.id, read.ID)
	must(t, e)
	revoked, e := f.service.RevokeData(ctx, f.actor, f.tenant, project.id, read.ID, md.RevokeInput{WriteMeta: f.meta(preview.Grant.Revision), PreviewHash: preview.Hash})
	must(t, e)
	fromText, toText = "2024-01-01T00:01:00Z", "2024-01-01T00:02:00Z"
	partial, e := f.service.GrantData(ctx, f.actor, f.tenant, project.id, md.GrantInput{WriteMeta: f.meta(revoked.AccessRevision), ChannelID: f.channel, ResourceGrantID: resourceGrant, Range: &md.Range{From: &fromText, To: &toText}, Capabilities: []string{"read", "history", "export"}})
	must(t, e)
	partFrom, partTo := "1704067260000000000", "1704067320000000000"
	part := dm.NanosecondRange{From: partFrom, To: &partTo}
	query := dm.AnalysisArtifactQuery{VersionID: version, Range: &part, Outputs: []string{"point"}, Limit: 1}
	page, e := s.AnalysisArtifactRows(ctx, project.actor, scope, artifact, query)
	must(t, e)
	if len(page.Items) != 1 || page.Items[0].Value == nil || *page.Items[0].Value != "5.000000000000000000" || !page.Version.Partial || page.Version.OutputRows != "" || page.Version.OutputRange.From != partFrom {
		t.Fatal("selected result was denied, changed or leaked full counts", page)
	}
	if _, e = s.AnalysisArtifactRows(ctx, project.actor, scope, artifact, dm.AnalysisArtifactQuery{VersionID: version, Outputs: []string{"point"}}); e == nil {
		t.Fatal("partial grant allowed entire artifact range")
	}
	if _, e = s.AnalysisArtifactRows(ctx, project.actor, scope, artifact, dm.AnalysisArtifactQuery{VersionID: version, Range: &part, Outputs: []string{"delta"}}); e == nil {
		t.Fatal("difference omitted the unauthorized original predecessor")
	}
	lineage, e := s.AnalysisArtifactLineage(ctx, project.actor, scope, artifact, query)
	must(t, e)
	line, e := json.Marshal(lineage["input_ranges"])
	must(t, e)
	if bytes.Contains(line, []byte(from)) || !bytes.Contains(line, []byte(partFrom)) || lineage["partial"] != true {
		t.Fatal("lineage exposed unrelated candidate ranges", string(line))
	}
	comparisonInput := dm.AnalysisCompareInput{WriteMeta: restoreMeta(""), LeftVersionID: version, RightVersionID: version, Range: &part, Outputs: []dm.AnalysisOutputMapping{{Left: "point", Right: "point"}}}
	compared, e := s.CompareAnalysis(ctx, project.actor, scope, comparisonInput, access.Meta{})
	must(t, e)
	assertRange := func(label string, run dm.Run) {
		t.Helper()
		if run.OutputRange.From != partFrom || run.OutputRange.To == nil || *run.OutputRange.To != partTo {
			t.Fatalf("%s disclosed whole-version bounds: %+v", label, run.OutputRange)
		}
	}
	assertRange("creation", compared)
	// Reproduce an immutable run created by the older implementation. Reads
	// must qualify its stored selections without rewriting historical evidence.
	must(t, pgx.BeginFunc(ctx, f.env.Pool, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `ALTER TABLE derived_runs DISABLE TRIGGER derived_run_immutable`); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, `UPDATE derived_runs SET output_range=int8range($2::bigint,$3::bigint,'[)') WHERE id=$1`, compared.ID, from, to); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `ALTER TABLE derived_runs ENABLE TRIGGER derived_run_immutable`)
		return e
	}))
	queued, e := s.Run(ctx, project.actor, scope, compared.ID)
	must(t, e)
	assertRange("queued historical read", queued)
	replayed, e := s.CompareAnalysis(ctx, project.actor, scope, comparisonInput, access.Meta{})
	must(t, e)
	if !replayed.Replayed || replayed.ID != compared.ID {
		t.Fatal("comparison request was not replayed", replayed)
	}
	assertRange("queued idempotent replay", replayed)
	_, e = f.service.RunTask(ctx, "partial-comparison", f.tenant, compared.Job.ID)
	must(t, e)
	compared, e = s.Run(ctx, project.actor, scope, compared.ID)
	must(t, e)
	assertRange("completed historical read", compared)
	if compared.Comparison == nil || len(compared.Comparison.Outputs) != 1 || compared.Comparison.Outputs[0].Unchanged != 1 || !compared.Comparison.Left.Partial || !compared.Comparison.Right.Partial || compared.Comparison.RangeChanged {
		t.Fatal("comparison did not retain selected qualification", compared)
	}
	for _, side := range []dm.AnalysisArtifactVersion{compared.Comparison.Left, compared.Comparison.Right} {
		assertRange("comparison side", dm.Run{OutputRange: side.OutputRange})
		if side.OutputRows != "" {
			t.Fatal("comparison exposed full version counts", side)
		}
	}
	listed, e := s.Runs(ctx, project.actor, scope, dm.RunFilter{})
	must(t, e)
	found := false
	for _, item := range listed.Items {
		if item.ID == compared.ID {
			found = true
			assertRange("run list", item)
		}
	}
	if !found {
		t.Fatal("authorized comparison missing from run list")
	}
	var persistedFrom string
	must(t, f.env.Pool.QueryRow(ctx, `SELECT lower(output_range)::text FROM derived_runs WHERE id=$1`, compared.ID).Scan(&persistedFrom))
	if persistedFrom != from {
		t.Fatal("read rewrote immutable comparison evidence", persistedFrom)
	}
	var prepared md.Download
	var exports []string
	for _, content := range []string{"results", "inputs"} {
		for _, format := range []string{"csv", "parquet"} {
			record, e := s.CreateAnalysisExport(ctx, project.actor, scope, artifact, dm.AnalysisExportInput{WriteMeta: restoreMeta(""), Content: content, VersionID: version, Range: &part, Outputs: []string{"point"}, Format: format}, access.Meta{})
			must(t, e)
			_, e = f.service.RunTask(ctx, "partial-analysis-export", f.tenant, record.Job.ID)
			must(t, e)
			manifest, e := s.AnalysisExportFiles(ctx, project.actor, scope, record.ID)
			must(t, e)
			if manifest.Rows != "1" {
				t.Fatal("partial export included hidden rows", manifest.Rows)
			}
			if content == "inputs" {
				verifyInputEvidenceFile(t, manifest, func(part string) (md.Download, error) {
					return s.AnalysisExportDownload(ctx, project.actor, scope, record.ID, part)
				}, []string{"5"})
			} else if format == "csv" {
				prepared, e = s.AnalysisExportDownload(ctx, project.actor, scope, record.ID, manifest.Files[0].Part)
				must(t, e)
				var buf bytes.Buffer
				must(t, prepared.Write(ctx, &buf))
				rows, e := csv.NewReader(&buf).ReadAll()
				must(t, e)
				if len(rows) != 2 || rows[1][13] != "5.000000000000000000" {
					t.Fatal("partial result export changed value", rows)
				}
			}
			exports = append(exports, record.ID)
		}
	}
	preview, e = f.service.PreviewRevokeData(ctx, f.actor, f.tenant, project.id, partial.ID)
	must(t, e)
	_, e = f.service.RevokeData(ctx, f.actor, f.tenant, project.id, partial.ID, md.RevokeInput{WriteMeta: f.meta(preview.Grant.Revision), PreviewHash: preview.Hash})
	must(t, e)
	if _, e = s.AnalysisArtifactRows(ctx, project.actor, scope, artifact, query); e == nil {
		t.Fatal("selected query bypassed current revocation")
	}
	var blocked bytes.Buffer
	if e = prepared.Write(ctx, &blocked); e == nil || blocked.Len() != 0 {
		t.Fatal("prepared selected export bypassed revocation", e, blocked.Len())
	}
	for _, id := range exports {
		if _, e = s.AnalysisExportFiles(ctx, project.actor, scope, id); e == nil {
			t.Fatal("selected export metadata bypassed revocation", id)
		}
	}
}
