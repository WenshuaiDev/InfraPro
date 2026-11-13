package derivedmonitoring_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	dm "github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/resources"
)

func TestDerivedConcurrentGraphEditsCannotCommitOppositeEdges(t *testing.T) {
	f := channelsSetup(t)
	firstActor := f.actor(f.admin)
	other := f.member(t, "SecondGraphEditor")
	secondActor := f.actor(other)
	// The second real account gets an explicit tenant role in this isolated
	// fixture. Different accounts ensure their account-row locks cannot provide
	// the serialization that the tenant graph itself is required to provide.
	tx, e := f.e.Pool.Begin(ctx)
	must(t, e)
	defer tx.Rollback(ctx)
	_, e = access.Lock(ctx, tx, true)
	must(t, e)
	var member, role string
	must(t, tx.QueryRow(ctx, `INSERT INTO tenant_memberships(tenant_id,account_id,joined_by,status_changed_by) VALUES($1,$2,$3,$3) RETURNING id::text`, f.scope.Tenant, other.Account.ID, firstActor.ID).Scan(&member))
	must(t, tx.QueryRow(ctx, `INSERT INTO roles(binding_scope,owner_tenant_id,name,name_normalized) VALUES('tenant',$1,'Graph editor','graph editor') RETURNING id::text`, f.scope.Tenant).Scan(&role))
	_, e = tx.Exec(ctx, `INSERT INTO role_permissions(role_id,permission_code,binding_scope) SELECT $1,p.permission_code,'tenant' FROM role_permissions p JOIN roles r ON r.id=p.role_id JOIN permission_catalog c ON c.code=p.permission_code WHERE r.builtin_key='tenant_admin' AND c.lifecycle='active' AND c.tenant_custom_role_assignable`, role)
	must(t, e)
	_, e = tx.Exec(ctx, `INSERT INTO tenant_member_roles(tenant_id,membership_id,role_id,assigned_by) VALUES($1,$2,$3,$4)`, f.scope.Tenant, member, role, firstActor.ID)
	must(t, e)
	must(t, tx.Commit(ctx))
	var rule dm.Rule
	must(t, json.Unmarshal(f.rule, &rule))
	rule.Nodes = []dm.Node{{ID: "result", Op: "abs", Inputs: []string{"input:x"}}}
	f.rule, e = json.Marshal(rule)
	must(t, e)
	definition := f.publish(t, f.create(t, "graph_abs").Definition)
	create := func(code, input string) dm.Channel {
		t.Helper()
		v, e := f.s.CreateChannel(ctx, firstActor, f.scope, dm.CreateChannelInput{WriteMeta: wm(""), Code: code, Name: code, Owner: dm.Owner{Kind: resources.Point, ID: f.point, FacilityID: f.facility}, DefinitionVersionID: definition.Version.ID, InputBindings: map[string]dm.InputBinding{"x": {ChannelID: input}}, Parameters: map[string]string{}}, access.Meta{})
		must(t, e)
		return v.Channel
	}
	base := create("BASE", f.direct)
	a, b := create("A", base.Resource.ID), create("B", base.Resource.ID)
	start := make(chan struct{})
	results := make(chan error, 2)
	for i, channel := range []dm.Channel{a, b} {
		actor, input := firstActor, b.Resource.ID
		if i == 1 {
			actor, input = secondActor, a.Resource.ID
		}
		go func() {
			<-start
			_, err := f.s.SaveChannelConfig(ctx, actor, f.scope, channel.Resource.ID, dm.SaveConfigInput{WriteMeta: wm(channel.ConfigRevision), BaseConfigID: channel.DraftConfigID, DefinitionVersionID: definition.Version.ID, InputBindings: map[string]dm.InputBinding{"x": {ChannelID: input}}, Parameters: map[string]string{}, ChangeKind: "ordinary"}, access.Meta{})
			results <- err
		}()
	}
	close(start)
	succeeded, rejected := 0, 0
	for range 2 {
		if err := <-results; err == nil {
			succeeded++
		} else {
			// A recursively bound output contract can reject this replacement
			// before graph validation: preserving business identity is also a
			// required outcome, not a reason to weaken the fixture's contract.
			var problem *dm.Error
			if !errors.As(err, &problem) || (problem.Code != "dependency_cycle" && problem.Code != "new_channel_required") {
				t.Fatal("unexpected graph edit rejection", err)
			}
			t.Log("opposite edge rejected", problem.Code)
			rejected++
		}
	}
	if succeeded > 1 || rejected < 1 {
		t.Fatal("opposite graph edits did not serialize", succeeded, rejected)
	}
	var oppositeEdges, configs int
	must(t, f.e.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM derived_dependencies d JOIN derived_channels c ON c.tenant_id=d.tenant_id AND c.draft_config_id=d.config_id WHERE d.tenant_id=$1 AND ((c.channel_id=$2 AND d.input_channel_id=$3) OR (c.channel_id=$3 AND d.input_channel_id=$2))),(SELECT count(*) FROM derived_channel_configs WHERE tenant_id=$1)`, f.scope.Tenant, a.Resource.ID, b.Resource.ID).Scan(&oppositeEdges, &configs))
	if oppositeEdges != succeeded || configs != 3+succeeded {
		t.Fatal("rejected graph edit left committed state", oppositeEdges, configs)
	}
}
