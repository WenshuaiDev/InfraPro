package projects_test

import (
	"slices"
	"testing"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/projects"
)

func TestProjectRoleSourceFilterBeforePagination(t *testing.T) {
	f := setup(t)
	pid := f.create(t, "source-filter", f.bid).ProjectID
	other := f.create(t, "source-other", f.bid).ProjectID
	first := f.role(t, pid, "source-first", []access.Code{access.ProjectProfileRead})
	second := f.role(t, pid, "source-second", []access.Code{access.ProjectProfileRead})
	f.role(t, other, "source-foreign", []access.Code{access.ProjectProfileRead})
	f.change(t, f.b, pid, first.ObjectID, access.OpProjectRoleStatus, projects.Change{Status: "disabled"})

	var ids []string
	cursor := ""
	for {
		page, err := f.s.Roles(ctx, f.actor(f.b), f.tid, pid, projects.Filter{Source: "custom", Limit: 1, Cursor: cursor}, false)
		must(t, err)
		if len(page.Items) != 1 || page.Items[0].OwnerProjectID != pid || page.Items[0].BuiltinKey != "" {
			t.Fatalf("custom filter page: %+v", page)
		}
		if slices.Contains(ids, page.Items[0].ID) {
			t.Fatal("duplicate role across cursors")
		}
		ids = append(ids, page.Items[0].ID)
		cursor = page.NextCursor
		if cursor == "" {
			break
		}
		if len(ids) > 2 {
			t.Fatal("filter leaked another project's role or a preset")
		}
	}
	if len(ids) != 2 || !slices.Contains(ids, first.ObjectID) || !slices.Contains(ids, second.ObjectID) {
		t.Fatal(ids)
	}
	filtered, err := f.s.Roles(ctx, f.actor(f.b), f.tid, pid, projects.Filter{Source: "custom", Status: "disabled", Query: "source-first", Limit: 1}, false)
	must(t, err)
	if len(filtered.Items) != 1 || filtered.Items[0].ID != first.ObjectID || filtered.NextCursor != "" {
		t.Fatal(filtered)
	}
	presets, err := f.s.Roles(ctx, f.actor(f.b), f.tid, pid, projects.Filter{Source: "preset", Limit: 100}, false)
	must(t, err)
	if len(presets.Items) == 0 {
		t.Fatal("missing presets")
	}
	for _, role := range presets.Items {
		if role.OwnerProjectID != "" || role.BuiltinKey == "" {
			t.Fatal(role)
		}
	}
	_, err = f.s.Roles(ctx, f.actor(f.b), f.tid, pid, projects.Filter{Source: "foreign"}, false)
	code(t, err, "validation_failed")
}
