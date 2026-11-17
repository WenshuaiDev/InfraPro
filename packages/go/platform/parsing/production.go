package parsing

import (
	"context"
	"errors"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
)

type IngressLink struct {
	SourceID      string `json:"source_id"`
	ProjectID     string `json:"source_project_id"`
	AssociationID string `json:"association_id"`
	Compatible    bool   `json:"compatible"`
	ErrorCode     string `json:"error_code,omitempty"`
}
type IngressImpact struct {
	Entries   []IngressLink `json:"entries"`
	Unplanned string        `json:"unplanned"`
	Unknown   bool          `json:"unknown"`
}
type AdoptionCompatibility interface {
	CheckAdoptionTx(context.Context, pgx.Tx, string, string, *Snapshot) (IngressImpact, error)
}
type ProductionSnapshot struct {
	AdoptionID       string    `json:"adoption_id"`
	DeviceID         string    `json:"device_id"`
	ProductVersionID string    `json:"product_version_id"`
	ConfigRevisionID string    `json:"config_revision_id"`
	Snapshot         Snapshot  `json:"configuration"`
	Entry            c.Entry   `json:"entry"`
	Mapping          []Mapping `json:"mapping"`
}

// ProductionSnapshotTx joins the caller's already locked and machine-authorized
// transaction. It reads the actual immutable adoption, never the product default.
func (s *Service) ProductionSnapshotTx(ctx context.Context, tx pgx.Tx, tenantID, deviceID, explicitConfig string) (ProductionSnapshot, error) {
	out := ProductionSnapshot{DeviceID: deviceID}
	var snapshot []byte
	e := tx.QueryRow(ctx, `SELECT a.id::text,a.product_version_id::text,a.config_revision_id::text,a.snapshot FROM parsing_device_adoptions a WHERE a.tenant_id=$1 AND a.device_id=$2 AND a.ended_at IS NULL AND a.mode<>'unconfigured'`, tenantID, deviceID).Scan(&out.AdoptionID, &out.ProductVersionID, &out.ConfigRevisionID, &snapshot)
	if e == pgx.ErrNoRows {
		return out, k.Problem("parser_unconfigured", "设备没有固定解析采用。", 409)
	}
	if e != nil {
		return out, e
	}
	if e = decode(snapshot, &out.Snapshot); e != nil {
		return out, e
	}
	if explicitConfig != "" {
		var product string
		e = tx.QueryRow(ctx, `SELECT r.snapshot,r.product_version_id::text FROM parsing_config_revisions r JOIN parsing_configs c ON c.id=r.config_id WHERE r.id=$1 AND (c.tenant_id IS NULL OR c.tenant_id=$2) AND (c.device_id IS NULL OR c.device_id=$3)`, explicitConfig, tenantID, deviceID).Scan(&snapshot, &product)
		if e != nil {
			return out, e
		}
		if product != out.ProductVersionID {
			return out, k.NotFound("parsing")
		}
		if e = decode(snapshot, &out.Snapshot); e != nil {
			return out, e
		}
		out.ConfigRevisionID = explicitConfig
	}
	m, e := methodTx(ctx, tx, out.Snapshot.Method.ID, out.Snapshot.Method.Version)
	if e != nil {
		return out, e
	}
	if e = allowedMethod(m, tenant(tenantID), false, false); e != nil {
		return out, e
	}
	if !m.Available || m.Entry.Method() != out.Snapshot.Method {
		return out, k.Problem("method_unavailable", "固定方法产物不可用。", 409)
	}
	out.Entry = m.Entry
	out.Mapping, e = mappingTx(ctx, tx, m.Entry, out.ProductVersionID, out.Snapshot.Bindings, out.Snapshot.Ignored, deviceID, false)
	return out, e
}
func (s *Service) ValidateProductionTx(ctx context.Context, tx pgx.Tx, tenantID string, p ProductionSnapshot) error {
	m, e := methodTx(ctx, tx, p.Entry.MethodID, p.Entry.MethodVersion)
	if e != nil {
		return e
	}
	if e = allowedMethod(m, tenant(tenantID), false, false); e != nil {
		return e
	}
	if !m.Available || m.Entry.Method() != p.Entry.Method() {
		return k.Problem("method_unavailable", "固定方法不可用。", 409)
	}
	var ok bool
	e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM devices d JOIN parsing_device_adoptions a ON a.device_id=d.id WHERE d.tenant_id=$1 AND d.id=$2 AND d.status='enabled' AND d.deleted_at IS NULL AND a.id=$3)`, tenantID, p.DeviceID, p.AdoptionID).Scan(&ok)
	if e != nil {
		return e
	}
	if !ok {
		return access.ErrDenied
	}
	return nil
}
func (s *Service) ExecuteFixed(ctx context.Context, req c.Request, entry c.Entry) ([]byte, c.Response, error) {
	var response c.Response
	if _, e := c.DecodeInput(req, entry); e != nil {
		return nil, response, e
	}
	b, e := s.call(ctx, "/v1/parse", req)
	if e == nil && (c.StrictJSON(b, &response) != nil || c.ValidateResponse(req, entry, response) != nil) {
		e = &serviceError{"invalid_response", false}
	}
	return b, response, e
}
func ErrorCode(e error) string { return codeOf(e) }
func Retryable(e error) bool   { var remote *serviceError; return errors.As(e, &remote) && remote.retry }

// ExecuteProduction shares validation and marks only a trusted internal workload.
type productionKey struct{}

func (s *Service) ExecuteProduction(ctx context.Context, req c.Request, entry c.Entry) ([]byte, c.Response, error) {
	return s.ExecuteFixed(context.WithValue(ctx, productionKey{}, true), req, entry)
}
