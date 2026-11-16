package parsing

import (
	"context"
	c "github.com/WenshuaiDev/InfraPro/packages/go/foundation/parsingcontract"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	k "github.com/WenshuaiDev/InfraPro/packages/go/platform/internal/devicekit"
	"github.com/jackc/pgx/v5"
	"strings"
)

const methodJSON = `to_jsonb(m)||jsonb_build_object('revision',m.revision::text)`

func methodTx(ctx context.Context, tx pgx.Tx, id, version string) (Method, error) {
	return readJSON[Method](tx.QueryRow(ctx, `SELECT `+methodJSON+` FROM parsing_method_versions m WHERE method_id=$1 AND method_version=$2`, id, version))
}
func allowedMethod(m Method, scope Scope, newUse, debug bool) error {
	if m.State == "blocked" {
		return k.Problem("method_blocked", "该版本已被平台禁止执行，历史依据仍保留。", 409)
	}
	if newUse && m.State != "enabled" || m.State == "pending" && !(scope.Entry == "platform" && debug) {
		return k.Problem("method_not_adoptable", "该版本当前不允许新采用。", 409)
	}
	return nil
}
func (s *Service) Methods(ctx context.Context, a access.Actor, scope Scope, f Filter) (k.Page[Method], error) {
	out := k.Page[Method]{Items: []Method{}}
	if e := f.Validate(); e != nil {
		return out, e
	}
	e := s.transact(ctx, a, scope, "methods", "list", false, func(tx pgx.Tx, auth k.Authorization) error {
		cur, e := k.DecodeCursor(f.Cursor, k.CursorContext(auth, f, "methods"))
		if e != nil {
			return e
		}
		rows, e := tx.Query(ctx, `SELECT `+methodJSON+` FROM parsing_method_versions m WHERE ($1 OR state<>'pending') AND ($2='' OR method_id ILIKE '%'||$2||'%' OR entry->>'name' ILIKE '%'||$2||'%') AND ($3='' OR state=$3) AND ($4::uuid IS NULL OR id>$4) ORDER BY id LIMIT $5`, scope.Entry == "platform", f.Query, f.State, k.Nullable(cur.ID), f.Limit+1)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			m, e := readJSON[Method](rows)
			if e != nil {
				return e
			}
			if len(out.Items) == f.Limit {
				out.NextCursor = k.Cursor{Scope: cur.Scope, ID: out.Items[len(out.Items)-1].ID}.Encode()
				break
			}
			m.ExecutionEnabled = s.options.Enabled && s.options.URL != "" && s.options.TokenFile != "" && s.objects != nil
			out.Items = append(out.Items, m)
		}
		return rows.Err()
	})
	return out, e
}
func (s *Service) Sync(ctx context.Context, a access.Actor, in Meta) (Result, error) {
	var out Result
	if e := validateMeta(&in, true); e != nil {
		return out, e
	}
	var replayed bool
	e := s.transact(ctx, a, platform(), "catalog", "sync", false, func(tx pgx.Tx, auth k.Authorization) error {
		var e error
		out, replayed, e = replay(ctx, tx, auth, "catalog.sync", in, in)
		return e
	})
	if e != nil || replayed {
		return out, e
	}
	b, remote := s.call(ctx, "/v1/catalog", nil)
	var catalog c.Catalog
	if remote == nil {
		if e = c.StrictJSON(b, &catalog); e != nil {
			remote = e
		} else if e = c.ValidateCatalog(catalog); e != nil {
			remote = e
		}
	}
	e = s.transact(ctx, a, platform(), "catalog", "sync", true, func(tx pgx.Tx, auth k.Authorization) error {
		if v, ok, e := replay(ctx, tx, auth, "catalog.sync", in, in); e != nil || ok {
			if ok {
				out = v
			}
			return e
		}
		failure := ""
		if remote != nil {
			failure = codeOf(remote)
			if failure == "invalid_response" {
				failure = "invalid_catalog"
			}
		}
		added := 0
		if failure == "" {
			for _, entry := range catalog.Entries {
				var digest, artifact string
				e := tx.QueryRow(ctx, `SELECT manifest_digest,artifact_digest FROM parsing_method_versions WHERE method_id=$1 AND method_version=$2`, entry.MethodID, entry.MethodVersion).Scan(&digest, &artifact)
				if e != nil && e != pgx.ErrNoRows {
					return e
				}
				if e == nil && (digest != entry.ManifestDigest || artifact != entry.ArtifactDigest) {
					failure = "version_conflict"
					break
				}
			}
		}
		if failure == "" {
			var existing []string
			rows, e := tx.Query(ctx, `SELECT method_id||'@'||method_version FROM parsing_method_versions`)
			if e != nil {
				return e
			}
			for rows.Next() {
				var key string
				if e = rows.Scan(&key); e != nil {
					rows.Close()
					return e
				}
				existing = append(existing, key)
			}
			e = rows.Err()
			rows.Close()
			if e != nil {
				return e
			}
			keys := map[string]bool{}
			for _, entry := range catalog.Entries {
				keys[entry.MethodID+"@"+entry.MethodVersion] = true
			}
			for _, key := range existing {
				if !keys[key] {
					failure = "incomplete_catalog"
					break
				}
			}
		}
		if failure == "" {
			for _, entry := range catalog.Entries {
				tag, e := tx.Exec(ctx, `INSERT INTO parsing_method_versions(method_id,method_version,entry,manifest_digest,artifact_digest) VALUES($1,$2,$3,$4,$5) ON CONFLICT(method_id,method_version) DO NOTHING`, entry.MethodID, entry.MethodVersion, k.JSON(entry), entry.ManifestDigest, entry.ArtifactDigest)
				if e != nil {
					return e
				}
				added += int(tag.RowsAffected())
			}
		}
		state := "succeeded"
		if failure != "" {
			state = "failed"
		}
		var saved any
		if remote == nil {
			saved = catalog
		}
		if e := tx.QueryRow(ctx, `INSERT INTO parsing_catalog_syncs(actor_id,state,error_code,catalog,difference) VALUES($1,$2,$3,$4,$5) RETURNING id::text`, a.ID, state, failure, k.JSON(saved), k.JSON(map[string]int{"added": added})).Scan(&out.ID); e != nil {
			return e
		}
		out.State = state
		out.ErrorCode = failure
		out.Revision = "1"
		return complete(ctx, tx, auth, "catalog.sync", in, in, out, nil, map[string]any{"id": out.ID, "state": state, "error_code": failure, "added": added})
	})
	return out, e
}
func copyResult(out, from *Result) error { *out = *from; return nil }
func (s *Service) SyncDetail(ctx context.Context, a access.Actor, id string) (map[string]any, error) {
	var out map[string]any
	e := s.transact(ctx, a, platform(), "catalog", "detail", false, func(tx pgx.Tx, _ k.Authorization) error {
		var e error
		out, e = readJSON[map[string]any](tx.QueryRow(ctx, `SELECT to_jsonb(s)-'catalog' FROM parsing_catalog_syncs s WHERE id=$1`, id))
		return e
	})
	return out, e
}
func (s *Service) SetMethodState(ctx context.Context, a access.Actor, id, version string, in StateInput) (Result, error) {
	var out Result
	if e := validateMeta(&in.Meta, true); e != nil {
		return out, e
	}
	if !strings.Contains("|pending|enabled|deprecated|blocked|", "|"+in.State+"|") {
		return out, k.Invalid("/state", "enum", "请选择有效版本状态。")
	}
	e := s.transact(ctx, a, platform(), "methods", "state", true, func(tx pgx.Tx, auth k.Authorization) error {
		m, e := methodTx(ctx, tx, id, version)
		if e != nil {
			return e
		}
		payload := []any{id, version, in}
		v, ok, e := replay(ctx, tx, auth, "methods.state", in.Meta, payload)
		if e != nil {
			return e
		}
		if ok {
			out = v
			return nil
		}
		if e = k.Expect(in.Revision, m.Revision); e != nil {
			return e
		}
		out.ID = m.ID
		if e = tx.QueryRow(ctx, `UPDATE parsing_method_versions SET state=$2,revision=revision+1,updated_at=clock_timestamp() WHERE id=$1 RETURNING revision::text`, m.ID, in.State).Scan(&out.Revision); e != nil {
			return e
		}
		return complete(ctx, tx, auth, "methods.state", in.Meta, payload, out, map[string]string{"state": m.State, "revision": m.Revision}, map[string]string{"state": in.State, "revision": out.Revision})
	})
	return out, e
}

func (s *Service) CatalogStatus(ctx context.Context, a access.Actor) (CatalogStatus, error) {
	out := CatalogStatus{State: "never_synced"}
	e := s.transact(ctx, a, platform(), "catalog", "status", false, func(tx pgx.Tx, _ k.Authorization) error {
		e := tx.QueryRow(ctx, `SELECT id::text,state,error_code,created_at,COALESCE((difference->>'added')::int,0) FROM parsing_catalog_syncs ORDER BY created_at DESC,id DESC LIMIT 1`).Scan(&out.ID, &out.State, &out.ErrorCode, &out.CreatedAt, &out.Added)
		if e == pgx.ErrNoRows {
			return nil
		}
		return e
	})
	return out, e
}
