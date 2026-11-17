package displayread

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"slices"
	"strconv"

	"github.com/jackc/pgx/v5"
)

func (s *Service) saveSectionsTx(ctx context.Context, tx pgx.Tx, id string, sections map[string][]json.RawMessage) error {
	for name, items := range sections {
		if !slices.Contains([]string{"coverage", "effective_windows"}, name) {
			return Invalid("/section", "未知查询分区。")
		}
		if _, e := tx.Exec(ctx, `DELETE FROM display_read_query_sections WHERE query_id=$1 AND section=$2`, id, name); e != nil {
			return e
		}
		if _, e := tx.CopyFrom(ctx, pgx.Identifier{"display_read_query_sections"}, []string{"query_id", "section", "ordinal", "data"}, pgx.CopyFromSlice(len(items), func(i int) ([]any, error) { return []any{id, name, i, items[i]}, nil })); e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) sectionPageTx(ctx context.Context, tx pgx.Tx, id, name, cursor string, limit int) (any, error) {
	if !slices.Contains([]string{"coverage", "effective_windows"}, name) {
		return nil, Invalid("/section", "未知查询分区。")
	}
	var count int64
	if e := tx.QueryRow(ctx, `SELECT count(*) FROM display_read_query_sections WHERE query_id=$1 AND section=$2`, id, name).Scan(&count); e != nil {
		return nil, e
	}
	ordinal := int64(0)
	if cursor != "" {
		var size int
		e := tx.QueryRow(ctx, `SELECT ordinal,page_size FROM display_read_section_cursors WHERE token=$1 AND query_id=$2 AND section=$3`, cursor, id, name).Scan(&ordinal, &size)
		if e == pgx.ErrNoRows || e == nil && size != limit {
			return nil, Problem("invalid_cursor", "分区游标与当前查询不符。", 409)
		}
		if e != nil {
			return nil, e
		}
	}
	rows, e := tx.Query(ctx, `SELECT data FROM display_read_query_sections WHERE query_id=$1 AND section=$2 AND ordinal>=$3 ORDER BY ordinal LIMIT $4`, id, name, ordinal, limit)
	if e != nil {
		return nil, e
	}
	items := []json.RawMessage{}
	for rows.Next() {
		var raw json.RawMessage
		if e = rows.Scan(&raw); e != nil {
			rows.Close()
			return nil, e
		}
		items = append(items, raw)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	next := ""
	more := ordinal+int64(len(items)) < count
	if more {
		e = tx.QueryRow(ctx, `INSERT INTO display_read_section_cursors(token,query_id,section,ordinal,page_size) VALUES($1,$2,$3,$4,$5) ON CONFLICT(query_id,section,ordinal,page_size) DO UPDATE SET query_id=EXCLUDED.query_id RETURNING token`, rand.Text()+rand.Text(), id, name, ordinal+int64(len(items)), limit).Scan(&next)
		if e != nil {
			return nil, e
		}
	}
	out := map[string]any{"section": name, "items": items, "query_id": id, "has_more": more, "next_cursor": next, "total": strconv.FormatInt(count, 10)}
	if len(Bytes(out)) > MaxResultBytes {
		return nil, Problem("budget_exceeded", "分区结果超过大小预算。", 429)
	}
	return out, nil
}
