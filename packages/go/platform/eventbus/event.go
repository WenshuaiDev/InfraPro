package eventbus

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"io"
	"strings"
	"time"
)

const MaxBytes = 16 << 10

var ErrBadReference = errors.New("invalid event fact reference")
var ErrDeferred = errors.New("event work not yet eligible")
var errEpoch = errors.New("inactive event epoch")

type Event struct {
	ID          string    `json:"event_id"`
	Type        string    `json:"type"`
	Version     int       `json:"version"`
	Scope       string    `json:"scope"`
	AggregateID string    `json:"aggregate_id"`
	Generation  int64     `json:"generation_or_position"`
	OccurredAt  time.Time `json:"occurred_at"`
	Epoch       string    `json:"restore_epoch"`
	Cause       string    `json:"cause_id,omitempty"`
	Trace       string    `json:"trace_id,omitempty"`
}

var consumerTopics = map[string]string{
	"identify": "ingestion.receipt.stored", "plan": "ingestion.units.ready", "parse": "ingestion.plan.ready",
	"summary": "monitoring.changed", "derived": "monitoring.changed", "query-cache": "monitoring.changed", "display-cache": "monitoring.changed",
	"task-index": "task.projection.ready", "notification": "notification.intent.ready", "audit": "audit.work.ready",
}
var families = []string{"ingestion", "monitoring", "auxiliary"}

func family(topic string) string {
	f, _, _ := strings.Cut(topic, ".")
	if f == "ingestion" || f == "monitoring" {
		return f
	}
	return "auxiliary"
}
func validID(s string) bool {
	u, e := uuid.Parse(s)
	return e == nil && u != uuid.Nil && u.String() == s
}
func decode(data []byte, epoch, expected string) (Event, string) {
	var e Event
	if len(data) > MaxBytes {
		return e, "bad_message"
	}
	// Zero is a valid task generation. Keep absence and JSON null distinct
	// from zero so a broker hint cannot omit its durable generation.
	var wire struct {
		*Event
		Generation *int64 `json:"generation_or_position"`
	}
	wire.Event = &e
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(&wire) != nil || wire.Generation == nil || d.Decode(new(any)) != io.EOF {
		return e, "bad_message"
	}
	e.Generation = *wire.Generation
	if e.Version != 1 {
		return e, "unsupported_version"
	}
	if !validID(e.ID) || e.Type != expected || e.Epoch != epoch || !validID(e.AggregateID) || e.Scope != "platform" && !validID(e.Scope) || e.Generation < 0 || e.OccurredAt.IsZero() || e.Cause != "" && !validID(e.Cause) || e.Trace != "" && !validID(e.Trace) {
		return e, "bad_reference"
	}
	return e, ""
}
func equal(a, b Event) bool {
	return a.ID == b.ID && a.Type == b.Type && a.Version == b.Version && a.Scope == b.Scope && a.AggregateID == b.AggregateID && a.Generation == b.Generation && a.Epoch == b.Epoch && a.OccurredAt.Equal(b.OccurredAt) && a.Cause == b.Cause && a.Trace == b.Trace
}
