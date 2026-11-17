package ingestion

import (
	"context"
	"encoding/json"
	i "github.com/WenshuaiDev/InfraPro/packages/go/foundation/ingestioncontract"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/migrations"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/parsing"
	"github.com/jackc/pgx/v5"
)

// Operator methods are intentionally not HTTP routes. The protected database and
// object-store configuration is required. Pause writers before a joint snapshot.
type OperatorManifest struct {
	Schema       int                      `json:"schema_version"`
	Methods      []c.Entry                `json:"measurements"`
	Capabilities i.Catalog                `json:"capabilities"`
	Objects      []parsing.RecoveryObject `json:"objects"`
	Receipts     []json.RawMessage        `json:"receipts"`
}

func (s *Service) OperatorManifest(ctx context.Context) (OperatorManifest, error) {
	out := OperatorManifest{Methods: []c.Entry{}, Objects: []parsing.RecoveryObject{}, Receipts: []json.RawMessage{}}
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		var version int64
		if e := tx.QueryRow(ctx, `SELECT COALESCE(MAX(version_id),0) FROM goose_db_version WHERE is_applied`).Scan(&version); e != nil {
			return e
		}
		if version != migrations.Version {
			return c.Fail("schema_incompatible")
		}
		out.Schema = int(version)
		for _, set := range []struct {
			query string
			take  func([]byte) error
		}{
			{`SELECT entry FROM parsing_method_versions ORDER BY method_id,method_version`, func(b []byte) error {
				var v c.Entry
				if e := json.Unmarshal(b, &v); e != nil {
					return e
				}
				out.Methods = append(out.Methods, v)
				return nil
			}},
			{`SELECT entry FROM parsing_identification_versions ORDER BY id`, func(b []byte) error {
				var v i.Identification
				if e := json.Unmarshal(b, &v); e != nil {
					return e
				}
				out.Capabilities.IdentificationMethods = append(out.Capabilities.IdentificationMethods, v)
				return nil
			}},
			{`SELECT entry FROM parsing_ingress_protocol_versions ORDER BY id`, func(b []byte) error {
				var v i.Protocol
				if e := json.Unmarshal(b, &v); e != nil {
					return e
				}
				out.Capabilities.IngressProtocols = append(out.Capabilities.IngressProtocols, v)
				return nil
			}},
			{`SELECT entry FROM parsing_ingress_compatibilities ORDER BY id`, func(b []byte) error {
				var v i.Compatibility
				if e := json.Unmarshal(b, &v); e != nil {
					return e
				}
				out.Capabilities.Compatibilities = append(out.Capabilities.Compatibilities, v)
				return nil
			}},
			{`SELECT jsonb_build_object('id',r.id,'relay_id',r.relay_id,'epoch',r.epoch,'receipt_id',r.receipt_id,'source_id',r.source_id,'source_sequence',r.source_sequence::text,'source_revision',r.source_revision::text,'envelope_digest',r.envelope_digest,'sha256',r.envelope->>'sha256','ack_state',o.state) FROM ingestion_receipts r LEFT JOIN ingestion_ack_outbox o ON o.receipt_id=r.id ORDER BY r.id`, func(b []byte) error { out.Receipts = append(out.Receipts, json.RawMessage(b)); return nil }},
		} {
			rows, e := tx.Query(ctx, set.query)
			if e != nil {
				return e
			}
			for rows.Next() {
				var b []byte
				if e = rows.Scan(&b); e != nil {
					rows.Close()
					return e
				}
				if e = set.take(b); e != nil {
					rows.Close()
					return e
				}
			}
			rows.Close()
			if e = rows.Err(); e != nil {
				return e
			}
		}
		rows, e := tx.Query(ctx, `SELECT id::text,object_key,sha256,byte_length FROM ingestion_objects WHERE state='available' ORDER BY id`)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var v parsing.RecoveryObject
			if e = rows.Scan(&v.ID, &v.Key, &v.SHA256, &v.Size); e != nil {
				return e
			}
			out.Objects = append(out.Objects, v)
		}
		return rows.Err()
	})
	return out, e
}
func (s *Service) VerifyRelease(ctx context.Context, directory string) (map[string]int, error) {
	m, e := s.OperatorManifest(ctx)
	if e != nil {
		return nil, e
	}
	_, e = i.VerifyPackage(directory, m.Methods, m.Capabilities)
	if e != nil {
		return nil, e
	}
	return map[string]int{"schema": m.Schema, "measurement_versions": len(m.Methods), "identification_versions": len(m.Capabilities.IdentificationMethods), "protocol_versions": len(m.Capabilities.IngressProtocols), "compatibilities": len(m.Capabilities.Compatibilities)}, nil
}
func (s *Service) VerifyObjects(ctx context.Context) (map[string]int, error) {
	m, e := s.OperatorManifest(ctx)
	if e != nil {
		return nil, e
	}
	out := map[string]int{"objects": len(m.Objects), "receipts": len(m.Receipts), "verified": 0, "missing_or_corrupt": 0}
	for _, v := range m.Objects {
		if _, e = s.readObject(ctx, object{ID: v.ID, Key: v.Key, Hash: v.SHA256, Size: v.Size}); e != nil {
			out["missing_or_corrupt"]++
		} else {
			out["verified"]++
		}
	}
	return out, nil
}

// Compare a protected cloud export with the restored owning database. A cloud
// receipt may be absent locally only if cloud payload and redelivery still exist.
func (s *Service) ReconcileManifest(ctx context.Context, cloud []json.RawMessage) (map[string]int, error) {
	out := map[string]int{"cloud_receipts": len(cloud), "matched": 0, "awaiting_transfer": 0, "gaps": 0, "conflicts": 0}
	e := s.access.Transact(ctx, func(tx pgx.Tx) error {
		for _, b := range cloud {
			var v struct {
				Relay   string `json:"relay_id"`
				Epoch   string `json:"spool_epoch"`
				Receipt string `json:"receipt_id"`
				Digest  string `json:"envelope_digest"`
				Payload bool   `json:"payload_present"`
				Ack     bool   `json:"acknowledged"`
			}
			if e := json.Unmarshal(b, &v); e != nil {
				return e
			}
			var digest string
			e := tx.QueryRow(ctx, `SELECT envelope_digest FROM ingestion_receipts WHERE relay_id=$1 AND epoch=$2 AND receipt_id=$3`, v.Relay, v.Epoch, v.Receipt).Scan(&digest)
			if e == pgx.ErrNoRows {
				if v.Payload && !v.Ack {
					out["awaiting_transfer"]++
				} else {
					out["gaps"]++
				}
			} else if e != nil {
				return e
			} else if digest != v.Digest {
				out["conflicts"]++
			} else {
				out["matched"]++
			}
		}
		return nil
	})
	return out, e
}
func (m OperatorManifest) Bytes() ([]byte, error) { return json.MarshalIndent(m, "", "  ") }
