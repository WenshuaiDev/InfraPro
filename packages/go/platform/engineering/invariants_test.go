package engineering_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	eng "github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	"github.com/jackc/pgx/v5"
)

func TestPublicationAndDependencyDisableHaveOneCommitOrder(t *testing.T) {
	f := setup(t)
	admin := f.actor(f.p)
	operator := f.member(t, "StateOperator")
	as := f.env.Service.Access()
	meta := access.Meta{Reason: "并发发布和停用"}
	role, e := as.CreateRole(ctx, admin, "目录状态维护", "", meta)
	must(t, e)
	role, e = as.ChangeRole(ctx, admin, role.ID, access.OpRoleSetPermissions, access.RoleChange{ExpectedVersion: role.Version, Permissions: []access.Code{access.EngineeringMetricRead, access.EngineeringMetricState}}, meta)
	must(t, e)
	account, e := as.Account(ctx, admin, operator.Account.ID)
	must(t, e)
	_, e = as.AssignRoles(ctx, admin, account.ID, account.Version, []string{role.ID}, meta)
	must(t, e)
	_, e = f.s.SaveCategory(ctx, admin, eng.CategoryInput{WriteMeta: f.wm(t), Key: "length", Name: "长度", Description: "空间长度"}, meta)
	must(t, e)
	unitContent := simple("毫米")
	unitContent.Unit = &eng.Unit{Symbol: "mm", Category: "length"}
	unit := f.publish(t, eng.UnitDefinition, f.create(t, eng.UnitDefinition, "mm", unitContent))
	unitScope := eng.Scope{Kind: eng.UnitDefinition}
	typeScope := eng.Scope{Kind: eng.TypeDefinition}
	// Name each real session without replacing its live session/permission check.
	named := func(a access.Actor, name string) access.Actor {
		check := a.Check
		a.Check = func(c context.Context, tx pgx.Tx, recent bool) error {
			if e := check(c, tx, recent); e != nil {
				return e
			}
			_, e := tx.Exec(c, `SELECT set_config('application_name',$1,true)`, name)
			return e
		}
		return a
	}
	for round, stopFirst := range []bool{true, false} {
		t.Run(fmt.Sprintf("disable_first_%v", stopFirst), func(t *testing.T) {
			if round > 0 {
				_, e = f.s.SetState(ctx, admin, unitScope, unit.DefinitionID, unit.VersionID, eng.StateInput{WriteMeta: f.wm(t), Revision: 2, Enabled: true}, meta)
				must(t, e)
			}
			c := simple("并发依赖类型")
			c.Fields = []eng.Field{{Key: "length", Name: "长度", Description: "空间长度", Shape: "decimal", UnitVersion: unit.VersionID}}
			draft := f.create(t, eng.TypeDefinition, fmt.Sprintf("race_%d", round), c)
			publishInput := eng.RevisionInput{WriteMeta: f.wm(t), Revision: draft.Revision}
			stopInput := eng.StateInput{WriteMeta: f.wm(t), Revision: 1 + round*2, Enabled: false}
			pubName, stopName := "publish_"+publishInput.RequestID, "stop_"+stopInput.RequestID
			lock, e := f.env.Pool.Begin(ctx)
			must(t, e)
			defer lock.Rollback(ctx)
			_, e = lock.Exec(ctx, `SELECT id FROM engineering_catalog_state WHERE id=1 FOR UPDATE`)
			must(t, e)
			published, stopped := make(chan error, 1), make(chan error, 1)
			publish := func() {
				_, e := f.s.Publish(ctx, named(admin, pubName), typeScope, draft.DefinitionID, publishInput, meta)
				published <- e
			}
			stop := func() {
				_, e := f.s.SetState(ctx, named(f.actor(operator), stopName), unitScope, unit.DefinitionID, unit.VersionID, stopInput, meta)
				stopped <- e
			}
			waiting := func(name string) {
				t.Helper()
				deadline := time.Now().Add(5 * time.Second)
				for time.Now().Before(deadline) {
					var blocked bool
					must(t, f.env.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE application_name=$1 AND wait_event_type='Lock')`, name).Scan(&blocked))
					if blocked {
						return
					}
					time.Sleep(10 * time.Millisecond)
				}
				t.Fatal("operation did not reach the catalog lock", name)
			}
			if stopFirst {
				go stop()
				waiting(stopName)
				go publish()
				waiting(pubName)
			} else {
				go publish()
				waiting(pubName)
				go stop()
				waiting(stopName)
			}
			must(t, lock.Commit(ctx))
			must(t, <-stopped)
			publishError := <-published
			if stopFirst {
				code(t, publishError, "dependency_unavailable")
			} else {
				must(t, publishError)
			}
			detail, e := f.s.Detail(ctx, admin, typeScope, draft.DefinitionID)
			must(t, e)
			if stopFirst && (detail.Draft == nil || len(detail.Versions) != 0) {
				t.Fatal("failed publication left partial history")
			}
			if !stopFirst && (detail.Draft != nil || len(detail.Versions) != 1 || detail.Versions[0].Usable) {
				t.Fatal("publication committed before stop must remain readable but unavailable")
			}
		})
	}
}

func TestUnitsMultipleTargetsAndStableKeys(t *testing.T) {
	f := setup(t)
	a := f.actor(f.p)
	units := map[string]eng.Result{}
	for _, category := range []string{"length", "pressure"} {
		_, e := f.s.SaveCategory(ctx, a, eng.CategoryInput{WriteMeta: f.wm(t), Key: category, Name: category, Description: category}, access.Meta{})
		must(t, e)
		c := simple(category)
		c.Unit = &eng.Unit{Category: category, Symbol: category}
		units[category] = f.publish(t, eng.UnitDefinition, f.create(t, eng.UnitDefinition, category, c))
	}
	m := simple("不兼容量纲")
	m.Metric = &eng.Metric{Shape: "decimal", Category: "length", StandardUnit: units["pressure"].VersionID, AllowedUnits: []string{units["pressure"].VersionID}}
	_, e := f.s.Create(ctx, a, eng.Scope{Kind: eng.MetricDefinition}, eng.CreateInput{WriteMeta: f.wm(t), Code: "bad_unit", Content: m}, access.Meta{})
	code(t, e, "definition_invalid")
	m.Metric.StandardUnit = units["length"].VersionID
	m.Metric.AllowedUnits = []string{units["length"].VersionID}
	metric := f.publish(t, eng.MetricDefinition, f.create(t, eng.MetricDefinition, "metric", m))
	c := simple("第一种设施")
	c.Fields = []eng.Field{{Key: "value", Name: "长度", Shape: "decimal", UnitVersion: units["length"].VersionID}}
	c.Components = []eng.Component{{Key: "layer", Name: "结构层"}}
	c.Relations = []eng.Relation{{Parent: "root", Child: "layer"}}
	first := f.publish(t, eng.TypeDefinition, f.create(t, eng.TypeDefinition, "first", c))
	second := f.publish(t, eng.TypeDefinition, f.create(t, eng.TypeDefinition, "second", simple("第二种设施")))
	template := simple("多目标模板")
	template.Template = &eng.Template{TypeVersions: []string{first.VersionID, second.VersionID}, Items: []eng.Item{{Key: "item", Name: "观测", Targets: []eng.Target{{TypeVersion: first.VersionID, Component: "layer"}}, Bindings: []eng.Binding{{MetricVersion: metric.VersionID, UnitVersion: units["length"].VersionID, ResourceKind: "observation_channel", MinCount: 1}}}}}
	draft := f.create(t, eng.MonitoringTemplate, "multiple", template)
	_, e = f.s.Publish(ctx, a, eng.Scope{Kind: eng.MonitoringTemplate}, draft.DefinitionID, eng.RevisionInput{WriteMeta: f.wm(t), Revision: draft.Revision}, access.Meta{})
	code(t, e, "definition_invalid")
	template.Template.Items[0].Targets = append(template.Template.Items[0].Targets, eng.Target{TypeVersion: second.VersionID, Component: "layer"})
	_, e = f.s.Save(ctx, a, eng.Scope{Kind: eng.MonitoringTemplate}, draft.DefinitionID, eng.SaveInput{WriteMeta: f.wm(t), Revision: draft.Revision, Content: template}, access.Meta{})
	code(t, e, "definition_invalid")
	template.Template.Items[0].Targets[1].Component = "root"
	draft, e = f.s.Save(ctx, a, eng.Scope{Kind: eng.MonitoringTemplate}, draft.DefinitionID, eng.SaveInput{WriteMeta: f.wm(t), Revision: draft.Revision, Content: template}, access.Meta{})
	must(t, e)
	f.publish(t, eng.MonitoringTemplate, draft)
	// Removing a key for one version does not allow later reuse with a changed meaning.
	for round := range 2 {
		draft, e = f.s.NewDraft(ctx, a, eng.Scope{Kind: eng.TypeDefinition}, first.DefinitionID, eng.NewDraftInput{RevisionInput: eng.RevisionInput{WriteMeta: f.wm(t), Revision: first.Revision}, VersionID: first.VersionID}, access.Meta{})
		must(t, e)
		if round == 0 {
			c.Fields = nil
		} else {
			c.Fields = []eng.Field{{Key: "value", Name: "长度", Shape: "text"}}
		}
		draft, e = f.s.Save(ctx, a, eng.Scope{Kind: eng.TypeDefinition}, first.DefinitionID, eng.SaveInput{WriteMeta: f.wm(t), Revision: draft.Revision, Content: c}, access.Meta{})
		must(t, e)
		if round == 0 {
			first = f.publish(t, eng.TypeDefinition, draft)
		} else {
			_, e = f.s.Publish(ctx, a, eng.Scope{Kind: eng.TypeDefinition}, first.DefinitionID, eng.RevisionInput{WriteMeta: f.wm(t), Revision: draft.Revision}, access.Meta{})
			code(t, e, "definition_invalid")
		}
	}
}
