package display

import (
	"context"
	"time"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/displayread"
	"github.com/jackc/pgx/v5"
)

// ReadBinding identifies an existing runtime session and its exact release and
// authority. It carries neither a runtime credential nor an arbitrary actor.
type ReadBinding struct {
	Session            string `json:"session"`
	Subject            string `json:"subject"`
	Tenant             string `json:"tenant"`
	Project            string `json:"project"`
	Instance           string `json:"instance"`
	Environment        string `json:"environment"`
	Package            string `json:"package"`
	Config             string `json:"config"`
	Generation         string `json:"generation"`
	Grant              string `json:"grant_revision"`
	TerminalGeneration string `json:"terminal_generation"`
}

func binding(v Session) ReadBinding {
	return ReadBinding{v.ID, v.SubjectID, v.Tenant, v.Project, v.InstanceID, v.Environment, v.PackageID, v.ConfigID, v.Generation, v.Authorization, v.TerminalGeneration}
}

type BoundRead struct {
	Binding   ReadBinding `json:"binding"`
	Authority string      `json:"authority"`
	Call      Call        `json:"call"`
	Deadline  time.Time   `json:"deadline"`
}
type ReadExecutor func(context.Context, BoundRead) (Envelope, error)

func (s *Service) WithReadExecutor(fn ReadExecutor) { s.remoteRead = fn }

func (s *Service) boundEvidence(ctx context.Context, b ReadBinding) (v Session, proof string, e error) {
	e = s.access.Transact(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SET TRANSACTION ISOLATION LEVEL REPEATABLE READ"); err != nil {
			return err
		}
		revision, err := access.Lock(ctx, tx, false)
		if err != nil {
			return err
		}
		v, err = scanSession(tx.QueryRow(ctx, sessionSelect+" WHERE s.id=$1", b.Session))
		if err != nil {
			return err
		}
		if binding(v) != b {
			return problem("reset_required", "读取依据已变化，请重新建立查询。", 409)
		}
		grant, _, err := s.checkSessionTx(ctx, tx, v, false)
		if err != nil {
			return err
		}
		// This request carries only the authority and safety proof. The query
		// owner computes the full data vector when it opens or rechecks a read.
		var data, resources, devices string
		err = tx.QueryRow(ctx, `SELECT
			coalesce((SELECT revision::text FROM monitoring_access_states WHERE tenant_id=$1 AND project_id=$2),'0'),
			policy.revision::text,
			coalesce((SELECT revision::text FROM project_device_access_states WHERE tenant_id=$1 AND project_id=$2),'0')
			FROM project_resource_policies policy CROSS JOIN device_classification_state classification
			WHERE policy.tenant_id=$1 AND policy.project_id=$2 AND classification.id=1`, v.Tenant, v.Project).Scan(&data, &resources, &devices)
		if err != nil {
			return err
		}
		vector := map[string]string{"authorization": revision, "data_grants": data, "resource_grants": resources, "device_grants": devices}
		authority, safety, err := readAuthorityStateTx(ctx, tx, v, grant, vector)
		if err != nil {
			return err
		}
		proof = hash([]string{authority, safety})
		return nil
	})
	return
}

// ExecuteBoundRead reconstructs authority from PG and uses the same request
// decoder, task admission, snapshot and result-delivery paths as local reads.
func (s *Service) ExecuteBoundRead(ctx context.Context, req BoundRead) (Envelope, error) {
	if !access.ValidID(req.Binding.Session) || len(req.Authority) != 64 {
		return Envelope{}, hidden()
	}
	ctx, cancel := context.WithDeadline(ctx, req.Deadline)
	defer cancel()
	started := time.Now()
	v, actual, e := s.boundEvidence(ctx, req.Binding)
	s.Measure("read_owner_authorization", started, e != nil)
	if e != nil {
		return Envelope{}, e
	}
	if req.Authority != actual {
		return Envelope{}, problem("reset_required", "读取依据已变化，请重新建立查询。", 409)
	}
	started = time.Now()
	out, e := s.queryLocal(ctx, v, req.Call)
	s.Measure("read_owner_execution", started, e != nil)
	return out, e
}

func (s *Service) queryRemote(ctx context.Context, v Session, call Call) (Envelope, error) {
	ctx, cancel := context.WithTimeout(ctx, displayread.SyncDeadline)
	defer cancel()
	b := binding(v)
	started := time.Now()
	_, proof, e := s.boundEvidence(ctx, b)
	s.Measure("read_request_authorization", started, e != nil)
	if e != nil {
		return Envelope{}, e
	}
	deadline, _ := ctx.Deadline()
	started = time.Now()
	out, e := s.remoteRead(ctx, BoundRead{b, proof, call, deadline})
	s.Measure("read_upstream", started, e != nil)
	if e != nil {
		return Envelope{}, e
	}
	started = time.Now()
	_, after, e := s.boundEvidence(ctx, b)
	s.Measure("read_delivery_authorization", started, e != nil)
	if e != nil {
		return Envelope{}, e
	}
	if after != proof {
		return Envelope{}, problem("reset_required", "读取依据已变化，请重新建立查询。", 409)
	}
	return out, nil
}
