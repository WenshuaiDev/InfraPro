package devices

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/classification"
	dr "github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	"github.com/jackc/pgx/v5"
	"slices"
)

func displayCategoryMatches(c *classification.Effective, f dr.DeviceFilters) bool {
	if c == nil {
		c = &classification.Effective{State: "unclassified"}
	}
	if f.CategoryID != "" {
		if f.CategoryScope == "descendants" {
			if !slices.Contains(c.Path, f.CategoryID) {
				return false
			}
		} else if c.CategoryID != f.CategoryID {
			return false
		}
	}
	switch f.CategoryGroup {
	case "sensor":
		return c.State == "classified" && c.Sensor != nil && *c.Sensor
	case "non_sensor":
		return c.State == "classified" && c.Sensor != nil && !*c.Sensor
	case "unclassified", "unavailable":
		return c.State == f.CategoryGroup
	}
	return true
}
func displayClassificationsTx(ctx context.Context, tx pgx.Tx, items []Device, f dr.DeviceFilters) ([]Device, error) {
	if f.DictionaryReleaseID != "" {
		release, e := classification.PublishedTx(ctx, tx, f.DictionaryReleaseID)
		if e != nil {
			return nil, e
		}
		nodes := map[string]classification.Node{}
		for _, n := range release.Content.Nodes {
			nodes[n.ID] = n
		}
		for i := range items {
			c := items[i].Classification
			if c == nil {
				c = &classification.Effective{State: "unclassified", Path: []string{}, NamePath: []string{}}
				items[i].Classification = c
			}
			c.DictionaryReleaseID = release.ID
			if c.CategoryID != "" {
				if n, ok := nodes[c.CategoryID]; ok {
					c.State = "classified"
					c.Name = n.Name
					c.Path = n.Path
					c.NamePath = n.NamePath
					c.Sensor = &n.Sensor
					c.CategoryStatus = n.Status
				} else {
					c.State = "unavailable"
					c.Name = ""
					c.Path = []string{}
					c.NamePath = []string{}
					c.Sensor = nil
					c.CategoryStatus = ""
				}
			}
		}
	}
	return slices.DeleteFunc(items, func(d Device) bool { return !displayCategoryMatches(d.Classification, f) }), nil
}
