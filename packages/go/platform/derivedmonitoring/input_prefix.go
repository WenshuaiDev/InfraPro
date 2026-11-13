package derivedmonitoring

import (
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/compute"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/planning"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoring"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
)

// Direct rank operators can locate their finite predecessor prefix in storage.
// Rank operators over computed nodes require complete earlier node state until
// a reusable state checkpoint is available; those reads remain budgeted and
// fail explicitly rather than producing a truncated-window numerical result.
func inputPrefixes(p *compute.Plan, cfg ChannelConfig, needs []monitoring.RawNeed, output planning.Range, sources ...*planning.Range) []monitoring.RawPrefix {
	out := []monitoring.RawPrefix{}
	prefixes := []planning.Predecessor{}
	var source *planning.Range
	if len(sources) > 0 {
		source = sources[0]
	}
	_, e := planning.Inputs(p, output, source, &prefixes)
	if e != nil {
		return out
	}
	for _, prefix := range prefixes {
		whole := prefix.WholeHistory
		channel := cfg.InputBindings[prefix.Input].ChannelID
		for _, need := range needs {
			if !whole && need.Channel != channel {
				continue
			}
			before := prefix.Before
			if whole {
				before = monitoringvalue.MinTime.UnixNano()
				if need.From != nil {
					before = *need.From
				}
			}
			out = append(out, monitoring.RawPrefix{Channel: need.Channel, Before: before, Count: prefix.Count, Qualities: prefix.Qualities, WholeHistory: whole})
		}
	}
	return out
}
