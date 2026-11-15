package tasks

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/auditrecord"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/taskrecord"
	"github.com/jackc/pgx/v5"
	"slices"
)

type Command struct {
	RequestID          string `json:"request_id"`
	Action             string `json:"action"`
	ExpectedGeneration string `json:"expected_generation"`
	ExpectedRevision   string `json:"expected_control_revision"`
	Reason             string `json:"reason"`
}
type CommandResult struct {
	TaskID       string `json:"task_id"`
	Generation   string `json:"generation"`
	CommandState string `json:"command_state"`
	State        string `json:"state"`
	Replayed     bool   `json:"replayed"`
}

func (c Command) Validate() error {
	if !access.ValidID(c.RequestID) || !slices.Contains([]string{"cancel", "retry"}, c.Action) {
		return access.Validation("command", "任务命令无效。")
	}
	for _, n := range []string{c.ExpectedGeneration, c.ExpectedRevision} {
		if _, e := taskrecord.Number(n, 1); e != nil {
			return access.Validation("revision", "需要有效执行代次和修订号。")
		}
	}
	if c.Action == "retry" || c.Reason != "" {
		return access.ValidReason(c.Reason)
	}
	return nil
}
func ValidateCommand(c Command, v View, a access.Actor) error {
	if e := c.Validate(); e != nil {
		return e
	}
	if c.Action == "retry" || v.Originator != a.ID {
		if e := access.ValidReason(c.Reason); e != nil {
			return e
		}
	}
	if v.Generation != c.ExpectedGeneration || v.ControlRevision != c.ExpectedRevision {
		return Problem("task_revision_changed", "任务状态已变化，请核对当前状态后重试。", 409)
	}
	for _, cap := range v.Actions {
		if cap.Kind == c.Action {
			if cap.Enabled {
				return nil
			}
			if cap.ReasonCode == "execution_principal_unavailable" {
				return Problem(cap.ReasonCode, "原执行主体已失效，请在原业务页面重新申请。", 409)
			}
			return Problem("task_action_denied", "当前条件不允许执行该操作。", 403)
		}
	}
	return Problem("task_action_denied", "该任务不支持此操作。", 403)
}
func SetCommandTx(ctx context.Context, tx pgx.Tx, a access.Actor, c Command, views ...View) error {
	if len(views) > 0 {
		v := views[0]
		operation, e := auditrecord.TransactionOperationTx(ctx, tx)
		if e != nil {
			return e
		}
		if e = auditrecord.SetCommandTx(ctx, tx, auditrecord.CommandContext{OperationID: operation, ActorID: a.ID, Initiator: v.Originator, RequestedBy: v.RequestedBy, Module: v.Source.Module, Kind: v.Source.Kind, RootID: v.Source.RootID, Generation: v.Generation, BeforeState: v.State, Action: c.Action, RequestID: c.RequestID, Scope: auditrecord.Scope{Kind: v.Scope.Kind, Tenant: v.Scope.Tenant, Project: v.Scope.Project}}); e != nil {
			return e
		}
	}
	_, e := tx.Exec(ctx, `SELECT set_config('infrapro.task_actor',$1,true),set_config('infrapro.task_reason',$2,true)`, a.ID, c.Reason)
	return e
}
func Outcome(v View, replayed bool) CommandResult {
	state := "applied"
	if v.State == "cancelling" {
		state = "requested"
	}
	return CommandResult{v.ID, v.Generation, state, v.State, replayed}
}
func (s *Service) Command(ctx context.Context, a access.Actor, id string, c Command) (CommandResult, error) {
	out := CommandResult{}
	if e := c.Validate(); e != nil {
		return out, e
	}
	var ref taskrecord.SourceRef
	e := s.transaction(ctx, a, func(tx pgx.Tx, _ string) error {
		v, e := s.getTx(ctx, tx, a, id)
		if e == nil && v.SourceMissing {
			return Problem("task_source_unavailable", "来源已清理，只能查看保留历史。", 503)
		}
		if e == nil {
			ref = v.Source
		}
		return e
	})
	if e != nil {
		return out, e
	}
	// No outer transaction: the source locks its own authority and row first and
	// checks the receipt before the precondition, so a lost response is replayable.
	return s.sources[ref.Module].Control(ctx, a, ref, c)
}
