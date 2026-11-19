package parsing

import (
	"context"
	"encoding/json"
	i "github.com/WenshuaiDev/InfraPro/packages/go/foundation/ingestioncontract"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
	"slices"
	"strconv"
)

type Capability struct {
	ID        string          `json:"id"`
	Entry     json.RawMessage `json:"entry"`
	State     string          `json:"state"`
	Revision  string          `json:"revision"`
	Available bool            `json:"available"`
	Digest    string          `json:"digest"`
}

var capabilityTables = map[string]string{"identification-methods": "parsing_identification_versions", "ingress-protocols": "parsing_ingress_protocol_versions", "ingress-compatibilities": "parsing_ingress_compatibilities"}

func capabilityJSON() string {
	return `jsonb_build_object('id',id,'entry',entry,'state',state,'revision',revision::text,'available',available,'digest',digest)`
}
func (s *Service) Capabilities(ctx context.Context, a access.Actor, scope Scope, kind string, f Filter) (k.Page[Capability], error) {
	out := k.Page[Capability]{Items: []Capability{}}
	table, ok := capabilityTables[kind]
	if !ok {
		return out, k.NotFound("capability")
	}
	if e := f.Validate(); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, kind, "list", false, func(tx pgx.Tx, auth k.Authorization) error {
		cur, e := k.DecodeCursor(f.Cursor, k.CursorContext(auth, f, kind))
		if e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT `+capabilityJSON()+` FROM `+table+` WHERE ($1 OR state<>'pending') AND ($2='' OR state=$2) AND ($3='' OR entry::text ILIKE '%'||$3||'%') AND ($4::uuid IS NULL OR id>$4) ORDER BY id LIMIT $5`, scope.Entry == "platform", f.State, f.Query, k.Nullable(cur.ID), f.Limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			v, e := readJSON[Capability](rows)
			if e != nil {
				return e
			}
			if len(out.Items) == f.Limit {
				out.NextCursor = k.Cursor{Scope: cur.Scope, ID: out.Items[len(out.Items)-1].ID}.Encode()
				break
			}
			out.Items = append(out.Items, v)
		}
		return rows.Err()
	})
	return out, e
}
func (s *Service) SyncIngestion(ctx context.Context, a access.Actor, in Meta) (Result, error) {
	var out Result
	if e := validateMeta(&in, true); e != nil {
		return out, e
	}
	var replayed bool
	e := s.transact(ctx, a, platform(), "ingestion-catalog", "sync", false, func(tx pgx.Tx, auth k.Authorization) error {
		var e error
		out, replayed, e = replay(ctx, tx, auth, "ingestion-catalog.sync", in, in)
		return e
	})
	if e != nil || replayed {
		return out, e
	}
	b, remote := s.call(ctx, "/v1/ingestion-capabilities", nil)
	var catalog i.Catalog
	if remote == nil {
		remote = c.StrictJSON(b, &catalog)
	}
	e = s.transact(ctx, a, platform(), "ingestion-catalog", "sync", true, func(tx pgx.Tx, auth k.Authorization) error {
		if v, ok, e := replay(ctx, tx, auth, "ingestion-catalog.sync", in, in); e != nil || ok {
			out = v
			return e
		}
		failure := ""
		if remote != nil {
			failure = "ingestion_catalog_unavailable"
		}
		measurement := c.Catalog{ContractVersion: 1, Entries: []c.Entry{}}
		rows, e := tx.Query(ctx, `SELECT entry FROM parsing_method_versions`)
		if e != nil {
			return e
		}
		for rows.Next() {
			var entry c.Entry
			var raw []byte
			if e = rows.Scan(&raw); e != nil {
				rows.Close()
				return e
			}
			if e = json.Unmarshal(raw, &entry); e != nil {
				rows.Close()
				return e
			}
			measurement.Entries = append(measurement.Entries, entry)
		}
		rows.Close()
		if e = rows.Err(); e != nil {
			return e
		}
		if failure == "" && i.ValidateCatalog(catalog, measurement) != nil {
			failure = "invalid_ingestion_catalog"
		}
		type row struct {
			kind, key, version, digest            string
			entry                                 any
			identification, identificationVersion string
			method, methodVersion                 string
		}
		list := []row{}
		for _, v := range catalog.IdentificationMethods {
			list = append(list, row{kind: "identification-methods", key: v.Method.ID, version: v.Method.Version, digest: v.Method.ManifestDigest, entry: v})
		}
		for _, v := range catalog.IngressProtocols {
			list = append(list, row{kind: "ingress-protocols", key: v.ID, version: v.Version, digest: v.Digest, entry: v, identification: v.Identification.ID, identificationVersion: v.Identification.Version})
		}
		for _, v := range catalog.Compatibilities {
			list = append(list, row{kind: "ingress-compatibilities", key: v.ID, version: strconv.FormatInt(v.Revision, 10), digest: v.Digest, entry: v, identification: v.ProtocolID, identificationVersion: v.ProtocolVersion, method: v.Measurement.ID, methodVersion: v.Measurement.Version})
		}
		if failure == "" {
			for kind, table := range capabilityTables {
				rows, e := tx.Query(ctx, `SELECT digest FROM `+table)
				if e != nil {
					return e
				}
				for rows.Next() {
					var digest string
					if e = rows.Scan(&digest); e != nil {
						rows.Close()
						return e
					}
					found := false
					for _, v := range list {
						if v.kind == kind && v.digest == digest {
							found = true
						}
					}
					if !found {
						failure = "incomplete_ingestion_catalog"
					}
				}
				rows.Close()
				if e = rows.Err(); e != nil {
					return e
				}
			}
		}
		if failure == "" {
			for _, v := range list {
				switch v.kind {
				case "identification-methods":
					_, e = tx.Exec(ctx, `INSERT INTO parsing_identification_versions(method_id,method_version,digest,entry) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, v.key, v.version, v.digest, k.JSON(v.entry))
				case "ingress-protocols":
					_, e = tx.Exec(ctx, `INSERT INTO parsing_ingress_protocol_versions(protocol_id,protocol_version,digest,entry,identification_id,identification_version) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`, v.key, v.version, v.digest, k.JSON(v.entry), v.identification, v.identificationVersion)
				case "ingress-compatibilities":
					_, e = tx.Exec(ctx, `INSERT INTO parsing_ingress_compatibilities(compatibility_id,compatibility_revision,digest,entry,protocol_id,protocol_version,method_id,method_version) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT DO NOTHING`, v.key, v.version, v.digest, k.JSON(v.entry), v.identification, v.identificationVersion, v.method, v.methodVersion)
				}
				if e != nil {
					return e
				}
			}
		}
		// Availability describes the currently verified complete capability package.
		// A failed sync preserves every historical identity and lifecycle state,
		// but must not advertise an absent or incomplete package as executable.
		for _, table := range capabilityTables {
			if _, e = tx.Exec(ctx, `UPDATE `+table+` SET available=$1 WHERE available<>$1`, failure == ""); e != nil {
				return e
			}
		}
		out.State = "succeeded"
		if failure != "" {
			out.State = "failed"
		}
		out.ErrorCode = failure
		out.Revision = "1"
		if e = tx.QueryRow(ctx, `INSERT INTO parsing_ingestion_catalog_syncs(actor_id,state,error_code,catalog) VALUES($1,$2,$3,$4) RETURNING id::text`, a.ID, out.State, failure, k.JSON(catalog)).Scan(&out.ID); e != nil {
			return e
		}
		return complete(ctx, tx, auth, "ingestion-catalog.sync", in, in, out, nil, map[string]string{"state": out.State, "error_code": failure})
	})
	return out, e
}
func (s *Service) SetCapabilityState(ctx context.Context, a access.Actor, kind, id string, in StateInput) (Result, error) {
	var out Result
	table, ok := capabilityTables[kind]
	if !ok {
		return out, k.NotFound("capability")
	}
	if e := validateMeta(&in.Meta, true); e != nil {
		return out, e
	}
	if !slices.Contains([]string{"pending", "enabled", "deprecated", "blocked"}, in.State) {
		return out, k.Invalid("/state", "enum", "请选择有效状态。")
	}
	e := s.transact(ctx, a, platform(), kind, "state", true, func(tx pgx.Tx, auth k.Authorization) error {
		v, e := readJSON[Capability](tx.QueryRow(ctx, `SELECT `+capabilityJSON()+` FROM `+table+` WHERE id=$1`, id))
		if e != nil {
			return e
		}
		payload := []any{kind, id, in}
		if prev, ok, e := replay(ctx, tx, auth, "capabilities.state", in.Meta, payload); e != nil || ok {
			out = prev
			return e
		}
		if e = k.Expect(in.Revision, v.Revision); e != nil {
			return e
		}
		if in.State == "enabled" && !v.Available {
			return k.Problem("artifact_unavailable", "该能力产物不可用。", 409)
		}
		out.ID = id
		out.State = in.State
		if e = tx.QueryRow(ctx, `UPDATE `+table+` SET state=$2,revision=revision+1 WHERE id=$1 RETURNING revision::text`, id, in.State).Scan(&out.Revision); e != nil {
			return e
		}
		return complete(ctx, tx, auth, "capabilities.state", in.Meta, payload, out, map[string]string{"state": v.State}, map[string]string{"state": in.State})
	})
	return out, e
}
func capabilityAllowed(state string, available, newUse bool) error {
	if state == "blocked" {
		return k.Problem("method_blocked", "接入能力已紧急阻断。", 409)
	}
	if !available {
		return k.Problem("artifact_unavailable", "接入产物不可用。", 409)
	}
	if state == "pending" || newUse && state != "enabled" {
		return k.Problem("method_not_adoptable", "接入能力未启用或已停止新采用。", 409)
	}
	return nil
}
func (s *Service) IngressProtocolTx(ctx context.Context, tx pgx.Tx, id, version string, newUse bool) (i.Protocol, error) {
	protocol, _, err := s.IngressIdentificationTx(ctx, tx, id, version, newUse)
	return protocol, err
}

// IngressIdentificationTx selects the protocol and its fixed identifier from
// the same catalog snapshot. Each still has its own availability and digest check.
func (s *Service) IngressIdentificationTx(ctx context.Context, tx pgx.Tx, id, version string, newUse bool) (i.Protocol, i.Identification, error) {
	var protocol i.Protocol
	var method i.Identification
	var protocolBytes, methodBytes []byte
	var protocolState, methodState string
	var protocolAvailable, methodAvailable bool
	err := tx.QueryRow(ctx, `SELECT p.entry,p.state,p.available,m.entry,m.state,m.available
		FROM parsing_ingress_protocol_versions p JOIN parsing_identification_versions m
		ON (m.method_id,m.method_version)=(p.identification_id,p.identification_version)
		WHERE (p.protocol_id,p.protocol_version)=($1,$2)`, id, version).Scan(&protocolBytes, &protocolState, &protocolAvailable, &methodBytes, &methodState, &methodAvailable)
	if err != nil {
		return protocol, method, err
	}
	if err = capabilityAllowed(protocolState, protocolAvailable, newUse); err != nil {
		return protocol, method, err
	}
	if err = json.Unmarshal(protocolBytes, &protocol); err != nil {
		return protocol, method, err
	}
	method, err = checkedIdentification(methodBytes, methodState, methodAvailable, protocol.Identification, newUse)
	return protocol, method, err
}

func (s *Service) IdentificationTx(ctx context.Context, tx pgx.Tx, ref i.Method, newUse bool) (i.Identification, error) {
	var raw []byte
	var state string
	var available bool
	err := tx.QueryRow(ctx, `SELECT entry,state,available FROM parsing_identification_versions WHERE method_id=$1 AND method_version=$2`, ref.ID, ref.Version).Scan(&raw, &state, &available)
	if err != nil {
		return i.Identification{}, err
	}
	return checkedIdentification(raw, state, available, ref, newUse)
}
func checkedIdentification(raw []byte, state string, available bool, ref i.Method, newUse bool) (i.Identification, error) {
	var out i.Identification
	if err := capabilityAllowed(state, available, newUse); err != nil {
		return out, err
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, err
	}
	if out.Method != ref {
		return out, k.Problem("version_conflict", "固定识别摘要不匹配。", 409)
	}
	return out, nil
}
func (s *Service) CompatibilityTx(ctx context.Context, tx pgx.Tx, p i.Protocol, demux *i.Method, view string, method c.Method, newUse bool) (i.Compatibility, error) {
	var out i.Compatibility
	rows, e := tx.Query(ctx, `SELECT entry,state,available FROM parsing_ingress_compatibilities WHERE protocol_id=$1 AND protocol_version=$2 AND method_id=$3 AND method_version=$4 ORDER BY compatibility_revision DESC`, p.ID, p.Version, method.ID, method.Version)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		var state string
		var available bool
		if e = rows.Scan(&raw, &state, &available); e != nil {
			return out, e
		}
		var v i.Compatibility
		if e = json.Unmarshal(raw, &v); e != nil {
			return out, e
		}
		if v.ProtocolDigest != p.Digest || v.InputView != view || v.ParseTransport != "tcp" || v.Measurement != method || (demux == nil) != (v.Demultiplexer == nil) {
			continue
		}
		if demux != nil && *demux != *v.Demultiplexer {
			continue
		}
		if e = capabilityAllowed(state, available, newUse); e != nil {
			return out, e
		}
		return v, nil
	}
	if e = rows.Err(); e != nil {
		return out, e
	}
	return out, k.Problem("ingress_method_incompatible", "缺少准确入口、输入视图及测量方法摘要的兼容声明。", 409)
}
func (s *Service) Identify(ctx context.Context, req i.Request, m i.Identification) ([]byte, i.Response, error) {
	var out i.Response
	if _, e := i.DecodeInput(req, m); e != nil {
		return nil, out, e
	}
	b, e := s.call(ctx, "/v1/identify", req)
	if e == nil && (c.StrictJSON(b, &out) != nil || i.ValidateResponse(req, m, out) != nil) {
		e = &serviceError{"invalid_identification_response", false}
	}
	return b, out, e
}
