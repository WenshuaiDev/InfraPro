package eventbus

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"slices"
	"strings"
	"time"
)

type Protocol struct {
	Epoch   string `json:"restore_epoch"`
	Enabled bool   `json:"enabled"`
	Frozen  bool   `json:"frozen"`
}

func ProtocolState(ctx context.Context, p *pgxpool.Pool) (Protocol, error) {
	var state Protocol
	e := p.QueryRow(ctx, `SELECT restore_epoch::text,enabled,frozen FROM event_protocol WHERE id=1`).Scan(&state.Epoch, &state.Enabled, &state.Frozen)
	return state, e
}
func startPositions(ctx context.Context, tx pgx.Tx, epoch string) error {
	_, err := tx.Exec(ctx, `INSERT INTO event_consumer_positions(consumer,restore_epoch,scope,position)
  SELECT consumer,$1,tenant_id,commit_sequence FROM event_destinations CROSS JOIN monitoring_tenant_states
  WHERE type='monitoring.changed' ON CONFLICT DO NOTHING`, epoch)
	return err
}

type Operation struct {
	Action    string
	Reason    string
	Role      string
	Mode      string
	Expected  int64
	Consumer  string
	ID        string
	Epoch     string
	SoftLimit int64
	HardLimit int64
}

// Operate serializes topology changes and records the effective PG operator and
// prior epoch. A repeated or stale cutover cannot retire the next generation.
func Operate(ctx context.Context, p *pgxpool.Pool, cmd Operation) (Protocol, error) {
	var out Protocol
	if l := len([]rune(strings.TrimSpace(cmd.Reason))); l < 1 || l > 300 {
		return out, errors.New("operational reason required")
	}
	err := pgx.BeginFunc(ctx, p, func(tx pgx.Tx) error {
		if e := tx.QueryRow(ctx, `SELECT restore_epoch::text,enabled,frozen FROM event_protocol WHERE id=1 FOR UPDATE`).Scan(&out.Epoch, &out.Enabled, &out.Frozen); e != nil {
			return e
		}
		original := out.Epoch
		if cmd.Epoch != "" && original != cmd.Epoch {
			return errEpoch
		}
		switch cmd.Action {
		case "enable":
			if !out.Enabled {
				if e := startPositions(ctx, tx, out.Epoch); e != nil {
					return e
				}
				if _, e := tx.Exec(ctx, `UPDATE event_protocol SET enabled=true WHERE id=1`); e != nil {
					return e
				}
				out.Enabled = true
			}
		case "freeze", "resume":
			frozen := cmd.Action == "freeze"
			if !frozen {
				var pending bool
				if e := tx.QueryRow(ctx, "SELECT restore_pending FROM event_protocol WHERE id=1").Scan(&pending); e != nil {
					return e
				}
				if pending {
					return errors.New("restore requires a new event epoch before resume")
				}
			}
			if _, e := tx.Exec(ctx, `UPDATE event_protocol SET frozen=$1 WHERE id=1`, frozen); e != nil {
				return e
			}
			out.Frozen = frozen
		case "limits":
			if cmd.SoftLimit < 1 || cmd.HardLimit <= cmd.SoftLimit {
				return errors.New("invalid outbox thresholds")
			}
			if _, e := tx.Exec(ctx, `UPDATE event_protocol SET soft_limit=$1,hard_limit=$2 WHERE id=1`, cmd.SoftLimit, cmd.HardLimit); e != nil {
				return e
			}
		case "switch":
			if !slices.Contains([]string{"split", "compat", "draining", "frozen"}, cmd.Mode) || cmd.Expected < 1 {
				return errors.New("dispatch mode and expected generation required")
			}
			tag, e := tx.Exec(ctx, `UPDATE dispatch_ownership SET mode=$1,generation=generation+1,changed_at=clock_timestamp(),
    freeze_position=(SELECT COALESCE(max(generation_or_position),0) FROM event_outbox WHERE type='monitoring.changed' AND restore_epoch=$4)
    WHERE role=$2 AND partition='all' AND generation=$3 AND restore_epoch=$4`, cmd.Mode, cmd.Role, cmd.Expected, out.Epoch)
			if e != nil {
				return e
			}
			if tag.RowsAffected() != 1 {
				return errors.New("dispatch generation changed or unknown role")
			}
		case "redrive":
			if topic := consumerTopics[cmd.Consumer]; topic == "" || !validID(cmd.ID) {
				return ErrBadReference
			}
			tag, e := tx.Exec(ctx, `UPDATE event_quarantine q SET state='redriven',updated_at=clock_timestamp()
    FROM event_work w WHERE q.consumer=$1 AND q.restore_epoch=$2 AND q.event_id=$3 AND q.state='open'
    AND (q.consumer,q.restore_epoch,q.event_id)=(w.consumer,w.restore_epoch,w.event_id) AND w.state='quarantined'`, cmd.Consumer, out.Epoch, cmd.ID)
			if e != nil {
				return e
			}
			if tag.RowsAffected() != 1 {
				return errors.New("no quarantined canonical fact")
			}
			if _, e = tx.Exec(ctx, `UPDATE event_work SET state='awaiting',attempts=0,fence=fence+1,available_at=clock_timestamp(),lease_until=NULL WHERE consumer=$1 AND restore_epoch=$2 AND event_id=$3`, cmd.Consumer, out.Epoch, cmd.ID); e != nil {
				return e
			}
		case "rotate-epoch":
			if !out.Frozen || cmd.Epoch == "" {
				return errors.New("freeze and supply expected epoch before rotation")
			}
			out.Epoch = uuid.NewString()
			if e := rotate(ctx, tx, original, out.Epoch); e != nil {
				return e
			}
		default:
			return fmt.Errorf("unknown event operation %q", cmd.Action)
		}
		_, e := tx.Exec(ctx, `INSERT INTO event_operations(action,restore_epoch,reason,details) VALUES($1,$2,$3,jsonb_strip_nulls(jsonb_build_object('role',NULLIF($4::text,''),'mode',NULLIF($5::text,''),'expected',NULLIF($6::bigint,0),'consumer',NULLIF($7::text,''),'event_id',NULLIF($8::text,''),'previous_epoch',$9::text,'soft_limit',NULLIF($10::bigint,0),'hard_limit',NULLIF($11::bigint,0))))`, cmd.Action, out.Epoch, cmd.Reason, cmd.Role, cmd.Mode, cmd.Expected, cmd.Consumer, cmd.ID, original, cmd.SoftLimit, cmd.HardLimit)
		return e
	})
	return out, err
}
func rotate(ctx context.Context, tx pgx.Tx, old, newEpoch string) error {
	// Keep the identity of completed work in the old epoch. Only outstanding
	// destinations receive a reference to the new epoch's canonical fact.
	_, err := tx.Exec(ctx, `WITH rebuilt AS (
 INSERT INTO event_outbox(producer,type,business_effect_key,restore_epoch,scope,aggregate_id,generation_or_position,occurred_at,version,cause_id,trace_id)
 SELECT producer,type,business_effect_key,$2,scope,aggregate_id,generation_or_position,occurred_at,version,event_id,trace_id
 FROM event_outbox o WHERE restore_epoch=$1 AND EXISTS(SELECT 1 FROM event_work w WHERE (w.restore_epoch,w.event_id)=(o.restore_epoch,o.event_id) AND w.state<>'done')
 RETURNING event_id,cause_id)
 INSERT INTO event_work(consumer,restore_epoch,event_id,state)
 SELECT w.consumer,$2,n.event_id,CASE WHEN w.state='quarantined' THEN 'quarantined' ELSE 'awaiting' END
 FROM event_work w JOIN rebuilt n ON n.cause_id=w.event_id WHERE w.restore_epoch=$1 AND state<>'done'`, old, newEpoch)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO event_quarantine(consumer,restore_epoch,event_id,aggregate_id,reason,attempts)
  SELECT q.consumer,$2,o.event_id,o.aggregate_id,q.reason,q.attempts FROM event_quarantine q JOIN event_outbox o ON o.cause_id=q.event_id AND o.restore_epoch=$2 WHERE q.restore_epoch=$1 AND q.state='open'`, old, newEpoch); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO event_consumer_positions(consumer,restore_epoch,scope,position) SELECT consumer,$2,scope,position FROM event_consumer_positions WHERE restore_epoch=$1`, old, newEpoch); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE dispatch_ownership SET generation=generation+1,restore_epoch=$1,changed_at=clock_timestamp()`, newEpoch); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM dispatch_leases`); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE event_pressure SET pending=0,oldest_at=NULL,broker_bytes=0,broker_limit_bytes=0,storage_level='normal',broker_sampled_at=NULL,sampled_at=clock_timestamp()`); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE event_protocol SET restore_epoch=$1,restore_pending=false WHERE id=1`, newEpoch)
	return err
}

// Prune removes a bounded page only when every destination committed an effect,
// the restore retention window passed and each ordered cursor covers the fact.
// Operational quarantine and epoch history are retained independently.
func Prune(ctx context.Context, p *pgxpool.Pool, retention time.Duration, limit int) (int64, error) {
	if retention <= 0 || limit < 1 || limit > 1024 {
		return 0, errors.New("invalid cleanup bounds")
	}
	var n int64
	e := p.QueryRow(ctx, "SELECT event_prune($1,$2)", int64(retention/time.Second), limit).Scan(&n)
	return n, e
}
