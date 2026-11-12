package resources

import (
	"errors"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/engineering"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/location"
)

type LinearReference = location.LinearReference
type Geographic = location.Geographic
type Projected = location.Projected
type Elevation = location.Elevation
type Local = location.Local
type Section = location.Section
type Position = location.Position

// Keep the existing resource error contract while sharing the pure validator.
func ValidatePosition(p *Position) error {
	err := location.ValidatePosition(p)
	var e *location.Error
	if !errors.As(err, &e) {
		return err
	}
	issues := []engineering.Issue{}
	for _, i := range e.Issues {
		issues = append(issues, engineering.Issue{Path: i.Path, Code: i.Code, Message: i.Message, Severity: "error"})
	}
	return &Error{Code: e.Code, Message: e.Message, Status: e.Status, Issues: issues}
}
