package devices

import (
	"context"
	"encoding/json"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
	"strings"
)

// Each side of a replacement/switch is qualified separately. Access to the old
// generation cannot expose a new generation, even in the old output's history.
func (s *Service) projectChanges(ctx context.Context, tx pgx.Tx, a access.Actor, auth k.Authorization, items []Change) error {
	ids := []string{}
	installations := []Installation{}
	object := func(raw json.RawMessage) map[string]json.RawMessage {
		var out map[string]json.RawMessage
		_ = json.Unmarshal(raw, &out)
		return out
	}
	stringField := func(m map[string]json.RawMessage, key string) string {
		var out string
		_ = json.Unmarshal(m[key], &out)
		return out
	}
	var addInstall func(json.RawMessage)
	addInstall = func(raw json.RawMessage) {
		m := object(raw)
		if len(m) == 0 {
			return
		}
		if sub, ok := m["installation"]; ok {
			addInstall(sub)
			addInstall(m["previous_installation"])
			return
		}
		var i Installation
		if json.Unmarshal(raw, &i) == nil && access.ValidID(i.ID) {
			installations = append(installations, i)
		}
	}
	for _, change := range items {
		for _, raw := range []json.RawMessage{change.Before, change.After} {
			m := object(raw)
			if strings.Contains(change.Operation, ":output:") {
				if id := stringField(m, "id"); access.ValidID(id) {
					ids = append(ids, id)
				}
			}
			if strings.Contains(change.Operation, ":source:") {
				if id := stringField(m, "output_id"); access.ValidID(id) {
					ids = append(ids, id)
				}
			}
			if strings.Contains(change.Operation, ":installation:") {
				addInstall(raw)
			}
		}
	}
	allowed := map[string]bool{}
	var e error
	if auth.Entry == "project" {
		rows, err := tx.Query(ctx, `SELECT output_id::text FROM project_device_grants WHERE tenant_id=$1 AND project_id=$2 AND kind='output' AND output_id=ANY($3::uuid[]) AND ended_at IS NULL`, auth.Tenant, auth.Project, ids)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id string
			if e = rows.Scan(&id); e != nil {
				rows.Close()
				return e
			}
			allowed[id] = true
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
	}
	// Use the original list position instead of IDs: a correction can contain two
	// snapshots of the same installation with different historical targets.
	projected := []Installation{}
	if len(installations) > 0 && s.resources != nil {
		projected, e = s.resources.ProjectInstallationsTx(ctx, tx, a, auth.Scope, installations)
		if e != nil {
			return e
		}
	}
	installIndex := 0
	var installSnapshot func(json.RawMessage) json.RawMessage
	installSnapshot = func(raw json.RawMessage) json.RawMessage {
		m := object(raw)
		if len(m) == 0 {
			return nil
		}
		if sub, ok := m["installation"]; ok {
			out := map[string]json.RawMessage{"installation": installSnapshot(sub)}
			if prev, ok := m["previous_installation"]; ok {
				out["previous_installation"] = installSnapshot(prev)
			}
			return k.JSON(out)
		}
		var i Installation
		if json.Unmarshal(raw, &i) != nil || !access.ValidID(i.ID) {
			return nil
		}
		index := installIndex
		installIndex++
		if index < len(projected) {
			return k.JSON(projected[index])
		}
		return k.JSON(map[string]any{"visibility": "restricted", "installed_at": i.InstalledAt, "uninstalled_at": i.UninstalledAt})
	}
	for index := range items {
		v := &items[index]
		// Tenant device rights do not imply rights to the installation target.
		// Keep tenant audit fields and unrelated snapshots, but qualify both sides
		// of every installation change just like the current installation view.
		if auth.Entry != "project" {
			if strings.Contains(v.Operation, ":installation:") {
				v.Before = installSnapshot(v.Before)
				v.After = installSnapshot(v.After)
			}
			continue
		}
		v.ActorName = ""
		v.Reason = ""
		v.Visibility = "full"
		project := func(raw json.RawMessage) json.RawMessage {
			m := object(raw)
			if len(m) == 0 {
				return nil
			}
			if strings.Contains(v.Operation, ":installation:") {
				return installSnapshot(raw)
			}
			if strings.Contains(v.Operation, ":output:") {
				id := stringField(m, "id")
				if id != "" && !allowed[id] {
					v.Visibility = "partial"
					return k.JSON(map[string]string{"visibility": "restricted"})
				}
				return projectHistoryFields(raw, v.Operation)
			}
			if strings.Contains(v.Operation, ":source:") {
				if !allowed[stringField(m, "output_id")] {
					v.Visibility = "partial"
					return k.JSON(map[string]any{"configured": true, "visibility": "restricted"})
				}
				safe := map[string]json.RawMessage{}
				for _, key := range []string{"id", "output_id", "facility_id", "point_id", "channel_id", "started_at", "ended_at", "revision", "output_snapshot", "channel_snapshot"} {
					if field, ok := m[key]; ok {
						safe[key] = field
					}
				}
				return k.JSON(safe)
			}
			return projectHistoryFields(raw, v.Operation)
		}
		v.Before = project(v.Before)
		v.After = project(v.After)
		if v.OutputID != nil && !allowed[*v.OutputID] {
			v.OutputID = nil
			v.Visibility = "partial"
		}
		if strings.Contains(v.Operation, ":source:") {
			v.BindingID = nil
		}
	}
	return nil
}
