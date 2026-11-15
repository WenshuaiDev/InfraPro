package monitoring

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/access"
	"github.com/jackc/pgx/v5"
)

// ImportQuery filters the authorized batch view, including redacted project views.
type ImportQuery struct {
	Cursor  string `json:"-"`
	Search  string `json:"q"`
	States  string `json:"states"`
	Formats string `json:"formats"`
	Sort    string `json:"sort"`
	Limit   int    `json:"limit"`
}

var importStates = []string{"uploading", "received", "validating", "needs_resolution", "ready", "preparing", "published", "completed_noop", "failed", "cancelled", "revoked"}

func normalizeImportQuery(q ImportQuery) (ImportQuery, error) {
	q.Search = strings.TrimSpace(q.Search)
	if !utf8.ValidString(q.Search) || utf8.RuneCountInString(q.Search) > 200 || strings.ContainsAny(q.Search, "\x00\r\n") {
		return q, invalid("q", "搜索内容最多 200 个字符，不能包含换行。")
	}
	for _, item := range []struct {
		value   *string
		allowed []string
		name    string
	}{{&q.States, importStates, "states"}, {&q.Formats, []string{"csv", "xlsx"}, "formats"}} {
		if *item.value == "" {
			continue
		}
		parts := strings.Split(*item.value, ",")
		if len(parts) > len(item.allowed) {
			return q, invalid(item.name, "筛选项过多。")
		}
		for _, part := range parts {
			if !slices.Contains(item.allowed, part) {
				return q, invalid(item.name, "筛选项无效。")
			}
		}
		slices.Sort(parts)
		*item.value = strings.Join(slices.Compact(parts), ",")
	}
	if q.Sort == "" {
		q.Sort = "newest"
	}
	if q.Sort != "newest" && q.Sort != "oldest" {
		return q, invalid("sort", "排序必须是 newest 或 oldest。")
	}
	if q.Limit == 0 {
		q.Limit = 50
	}
	if !slices.Contains([]int{10, 20, 25, 50, 100}, q.Limit) {
		return q, invalid("limit", "每页数量须为 10、20、25、50 或 100。")
	}
	return q, nil
}

type importCursor struct {
	listBoundary
	Key           string `json:"key"`
	Authorization string `json:"authorization"`
}

func importCursorKey(a access.Actor, scope Scope, q ImportQuery) string {
	h := sha256.Sum256(encodeJSON([]any{a.ID, scope, q}))
	return hex.EncodeToString(h[:])
}

func parseImportCursor(q ImportQuery, key string) (importCursor, error) {
	var out importCursor
	if q.Cursor == "" {
		return out, nil
	}
	if !strings.HasPrefix(q.Cursor, "imports.v1.") {
		// Existing unfiltered bookmarks keep working during the UI transition.
		if q.Search != "" || q.States != "" || q.Formats != "" || q.Sort != "newest" || q.Limit != 50 {
			return out, importCursorStale()
		}
		boundary, err := parseListCursor(q.Cursor)
		out.listBoundary = boundary
		return out, err
	}
	if len(q.Cursor) > 2048 {
		return out, invalid("cursor", "无效分页位置。")
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(q.Cursor, "imports.v1."))
	if err != nil || json.Unmarshal(raw, &out) != nil || out.At == nil || out.At.IsZero() || !access.ValidID(out.ID) || out.Authorization == "" {
		return importCursor{}, invalid("cursor", "无效分页位置。")
	}
	if out.Key != key {
		return importCursor{}, importCursorStale()
	}
	return out, nil
}

func importCursorStale() error {
	return problem("monitoring_import_cursor_stale", "筛选或授权已变化，请返回第一页重新查询。", 409)
}

func matchesImport(b ImportBatch, q ImportQuery) bool {
	if q.States != "" && !slices.Contains(strings.Split(q.States, ","), b.State) {
		return false
	}
	if q.Search != "" && !strings.Contains(strings.ToLower(b.Name), strings.ToLower(q.Search)) && !strings.Contains(strings.ToLower(b.ID), strings.ToLower(q.Search)) {
		return false
	}
	// Never infer a hidden source format from a partially authorized batch.
	if q.Formats != "" && (b.ScopeClipped || !slices.Contains(strings.Split(q.Formats, ","), strings.TrimPrefix(strings.ToLower(filepath.Ext(b.Name)), "."))) {
		return false
	}
	return true
}

func (s *Service) QueryImports(ctx context.Context, a access.Actor, scope Scope, raw ImportQuery) (ImportPage, error) {
	out := ImportPage{Items: []ImportBatch{}}
	q, err := normalizeImportQuery(raw)
	if err != nil {
		return out, err
	}
	key := importCursorKey(a, scope, q)
	cursor, err := parseImportCursor(q, key)
	if err != nil {
		return out, err
	}
	boundary := cursor.listBoundary
	var revision string
	order, comparator := "DESC", "<"
	if q.Sort == "oldest" {
		order, comparator = "ASC", ">"
	}
	// Each scan is bounded; advance through invisible candidates until the visible
	// page is full. A cursor is only ever made from a returned, authorized batch.
	for len(out.Items) <= q.Limit {
		candidates := []ImportBatch{}
		full := map[string]bool{}
		err = s.transact(ctx, a, scope, "imports.list", false, false, func(tx pgx.Tx, auth authorization) error {
			current := authorizationVersion(auth)
			if cursor.Authorization != "" && cursor.Authorization != current {
				return importCursorStale()
			}
			if revision != "" && revision != current {
				return revisionChanged()
			}
			revision = current
			rows, e := tx.Query(ctx, batchSelect+` WHERE tenant_id=$1
 AND ($2='' OR project_id::text=$2 OR (state IN ('published','revoked') AND EXISTS(SELECT 1 FROM monitoring_data_grants d JOIN project_resource_grants g ON g.id=d.resource_grant_id JOIN project_facility_links l ON l.id=g.link_id WHERE d.tenant_id=$1 AND d.project_id::text=$2 AND d.revoked_at IS NULL AND 'read'=ANY(d.capabilities) AND g.ended_at IS NULL AND l.status='active' AND (semantic_snapshot->'actual_coverage') ? d.channel_id::text)))
 AND ($3::timestamptz IS NULL OR (created_at,id)`+comparator+`($3::timestamptz,$4::uuid))
 AND ($5='' OR state=ANY(string_to_array($5,',')))
 AND ($6='' OR position(lower($6) in lower(original_name))>0 OR position(lower($6) in id::text)>0 OR ($2<>'' AND position(lower($6) in '共享批次（当前授权范围）')>0))
 AND ($7='' OR lower(substring(original_name from '\.([^.]+)$'))=ANY(string_to_array($7,',')))
 ORDER BY created_at `+order+`,id `+order+` LIMIT 100`, scope.Tenant, scope.Project, boundary.At, nullable(boundary.ID), q.States, q.Search, q.Formats)
			if e != nil {
				return e
			}
			for rows.Next() {
				b, e := scanBatch(rows)
				if e != nil {
					rows.Close()
					return e
				}
				candidates = append(candidates, b)
			}
			e = rows.Err()
			rows.Close()
			if e != nil {
				return e
			}
			for _, b := range candidates {
				e = batchVisibilityTx(ctx, tx, auth, b)
				if e == nil {
					full[b.ID] = true
				} else if !isHidden(e) {
					return e
				}
			}
			return nil
		})
		if err != nil {
			return ImportPage{}, err
		}
		for _, candidate := range candidates {
			b := candidate
			if !full[b.ID] {
				if scope.Entry != "project" || !slices.Contains([]string{"published", "revoked"}, b.State) {
					continue
				}
				// A type filter excludes partial views without reading their data.
				if q.Formats != "" {
					continue
				}
				b, err = s.partialImport(ctx, a, scope, b.ID)
				if isHidden(err) {
					continue
				}
				if err != nil {
					return ImportPage{}, err
				}
			}
			if matchesImport(b, q) {
				out.Items = append(out.Items, b)
			}
			if len(out.Items) > q.Limit {
				break
			}
		}
		if len(candidates) < 100 || len(out.Items) > q.Limit {
			break
		}
		last := candidates[len(candidates)-1]
		boundary = listBoundary{At: &last.CreatedAt, ID: last.ID}
	}
	if len(out.Items) > q.Limit {
		out.Items = out.Items[:q.Limit]
		last := out.Items[len(out.Items)-1]
		out.NextCursor = "imports.v1." + base64.RawURLEncoding.EncodeToString(encodeJSON(importCursor{listBoundary{&last.CreatedAt, last.ID}, key, revision}))
	}
	err = s.transact(ctx, a, scope, "imports.list", false, false, func(_ pgx.Tx, auth authorization) error {
		if authorizationVersion(auth) != revision {
			return revisionChanged()
		}
		return nil
	})
	if err != nil {
		return ImportPage{}, err
	}
	return out, nil
}
