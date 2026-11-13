package observation

import (
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
	"time"
)

var MinTime = monitoringvalue.MinTime
var MaxTime = monitoringvalue.MaxTime

func CheckTime(t time.Time) error              { return monitoringvalue.CheckTime(t) }
func ParseTime(text string) (time.Time, error) { return monitoringvalue.ParseTime(text) }
func ParseLocal(text, layout, zone string) (time.Time, error) {
	return monitoringvalue.ParseLocal(text, layout, zone)
}
