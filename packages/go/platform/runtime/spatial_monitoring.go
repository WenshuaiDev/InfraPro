package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/spatial"
	"sort"
	"sync"
	"time"
)

type spatialMonitoring struct{ service *monitoring.Service }
type spatialChannelSummary struct {
	Channel   monitoring.MonitoringChannel `json:"channel"`
	Selection monitoring.Selection         `json:"selection"`
	Latest    monitoring.AnalyticResult    `json:"latest"`
}
type spatialOwnerSummary struct {
	Owner      spatial.ObjectRef       `json:"owner"`
	Channels   []spatialChannelSummary `json:"channels"`
	NextCursor string                  `json:"next_cursor"`
}

func monitoringRestricted(e error) bool {
	var me *monitoring.Error
	var de *derivedmonitoring.Error
	var ae *access.Error
	return errors.Is(e, access.ErrDenied) || errors.As(e, &me) && (me.Status == 403 || me.Status == 404) || errors.As(e, &de) && (de.Status == 403 || de.Status == 404) || errors.As(e, &ae) && (ae.Status == 403 || ae.Status == 404)
}
func (r spatialMonitoring) Summaries(ctx context.Context, a access.Actor, scope spatial.Scope, in spatial.ObjectQuery) (json.RawMessage, error) {
	items := []spatialOwnerSummary{}
	ms := monitoring.Scope{Entry: scope.Entry, Tenant: scope.Tenant, Project: scope.Project}
	total := 0
	for _, ref := range in.Objects {
		item := spatialOwnerSummary{Owner: ref, Channels: []spatialChannelSummary{}}
		page, e := r.service.ChannelDirectory(ctx, a, ms, monitoring.ChannelNavigationQuery{OwnerKind: ref.Kind, OwnerID: ref.ID})
		if monitoringRestricted(e) {
			items = append(items, item)
			continue
		}
		if e != nil {
			return nil, e
		}
		preferred := in.MainChannels[ref.Kind+":"+ref.ID]
		if preferred != "" {
			exact, err := r.service.ChannelDirectory(ctx, a, ms, monitoring.ChannelNavigationQuery{OwnerKind: ref.Kind, OwnerID: ref.ID, Channel: preferred})
			if err != nil && !monitoringRestricted(err) {
				return nil, err
			}
			if len(exact.Items) == 1 {
				found := false
				for _, ch := range page.Items {
					if ch.ID == preferred {
						found = true
						break
					}
				}
				if !found {
					page.Items = append(page.Items, exact.Items[0])
				}
			}
		}
		// Read direct observations first and use a bounded four-way I/O batch.
		// Each channel retains its own authorization and snapshot; a denied
		// derived input cannot mask the other authorized channels.
		sort.SliceStable(page.Items, func(i, j int) bool {
			if (page.Items[i].ID == preferred) != (page.Items[j].ID == preferred) {
				return page.Items[i].ID == preferred
			}
			if page.Items[i].SourceKind != page.Items[j].SourceKind {
				return page.Items[i].SourceKind == "direct"
			}
			return page.Items[i].ID < page.Items[j].ID
		})
		for offset := 0; offset < len(page.Items); offset += 4 {
			if len(item.Channels) >= 10 || total >= 100 {
				if len(item.Channels) > 0 {
					item.NextCursor = item.Channels[len(item.Channels)-1].Channel.ID
				}
				break
			}
			end := offset + 4
			if end > len(page.Items) {
				end = len(page.Items)
			}
			batch := make([]*spatialChannelSummary, end-offset)
			errs := make([]error, len(batch))
			var group sync.WaitGroup
			for i, ch := range page.Items[offset:end] {
				group.Add(1)
				go func(i int, ch monitoring.MonitoringChannel) {
					defer group.Done()
					selection, e := r.service.Select(ctx, a, ms, monitoring.SelectionInput{Channels: []string{ch.ID}})
					if e != nil {
						if !monitoringRestricted(e) {
							errs[i] = e
						}
						return
					}
					latest, e := r.service.Analytics(ctx, a, ms, "latest", monitoring.QueryInput{Selection: selection.ID, AllHistory: true, Timezone: "UTC", Status: "active", Limit: 1})
					if e != nil {
						if !monitoringRestricted(e) {
							errs[i] = e
						}
						return
					}
					ch.Position = nil
					batch[i] = &spatialChannelSummary{ch, selection, latest}
				}(i, ch)
			}
			group.Wait()
			for i, value := range batch {
				if errs[i] != nil {
					return nil, errs[i]
				}
				if value != nil {
					if len(item.Channels) >= 10 || total >= 100 {
						item.NextCursor = item.Channels[len(item.Channels)-1].Channel.ID
						break
					}
					item.Channels = append(item.Channels, *value)
					total++
				}
			}
		}
		if item.NextCursor == "" && page.NextCursor != "" && len(item.Channels) > 0 {
			item.NextCursor = item.Channels[len(item.Channels)-1].Channel.ID
		}
		items = append(items, item)
	}
	return k.JSON(map[string]any{"items": items, "refreshed_at": time.Now().UTC(), "snapshot_note": "各通道保持自身查询快照，不表示全景同刻采样。"}), nil
}

type spatialArtifacts struct{ service *derivedmonitoring.Service }

func (r spatialArtifacts) Related(ctx context.Context, a access.Actor, scope spatial.Scope, in spatial.RelatedInput) (json.RawMessage, error) {
	out, e := r.service.RelatedArtifacts(ctx, a, derivedmonitoring.Scope{Entry: scope.Entry, Tenant: scope.Tenant, Project: scope.Project}, derivedmonitoring.RelatedArtifactQuery{OwnerKind: in.Owner.Kind, OwnerID: in.Owner.ID, Descendants: in.Descendants, From: in.From, To: in.To, Cursor: in.Cursor, Limit: in.Limit})
	if monitoringRestricted(e) {
		return k.JSON(map[string]any{"items": []any{}, "next_cursor": ""}), nil
	}
	if e != nil {
		return nil, e
	}
	return k.JSON(out), nil
}
