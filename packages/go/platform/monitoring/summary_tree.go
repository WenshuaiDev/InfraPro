package monitoring

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/minio/minio-go/v7"
)

const minuteNS int64 = 60_000_000_000
const minimumMinute int64 = -36_816_480 // 1900-01-01, aligned to UTC minute.
const summaryObjectLimit = 256 << 20

var errSummaryPending = errors.New("summary_not_ready")
var errQueryBudget = errors.New("interactive_query_budget")

// Three radix-256 branch levels index sparse blocks of 256 UTC minutes. Empty
// minutes have no objects. Immutable content-addressed paths retain old roots.
type summaryRef struct {
	Object string
	Total  summaryTotals
}
type leafContribution struct {
	Active  bool
	Minutes map[string]summaryTotals
}
type summaryNode struct {
	Version  int
	Children map[string]summaryRef       `json:",omitempty"`
	Inputs   map[string]leafContribution `json:",omitempty"`
	Minutes  map[string]summaryTotals    `json:",omitempty"`
	Total    summaryTotals
}
type chunkContributions map[string]map[string]map[string]summaryTotals // channel/block/minute
type summaryRoot struct {
	Version         int
	ContentRevision string
	Root            summaryRef
}
type summaryChange struct {
	Chunk, Object string
	Active        bool
}
type treeBuilder struct {
	storage *Storage
	tenant  string
	nodes   map[string]*summaryNode
	writes  int
}

func minuteKey(ns int64) int64 {
	q := ns / minuteNS
	if ns < 0 && ns%minuteNS != 0 {
		q--
	}
	return q - minimumMinute
}
func minuteWindow(key int64) Window {
	lo := (key + minimumMinute) * minuteNS
	var hi *int64
	if lo <= int64(1<<63-1)-minuteNS {
		n := lo + minuteNS
		hi = &n
	}
	return Window{From: &lo, To: hi}
}
func summaryRootKey(tenant, channel, revision string) string {
	return tenant + "/summaries/v1/roots/" + channel + "/" + revision + ".json"
}
func (s *Storage) summaryObject(ctx context.Context, key string, out any, hashed bool) error {
	// Reading the entire immutable index validates existence and reports the
	// storage error before any result is returned. An initial Stat would issue
	// an additional network request for every channel in the fixed selection.
	object, e := s.openObject(ctx, key)
	if e != nil {
		return e
	}
	defer object.Close()
	var input io.Reader = measureObjectRead(ctx, object)
	if hashed {
		z, e := gzip.NewReader(input)
		if e != nil {
			return e
		}
		defer z.Close()
		input = z
	}
	raw, e := io.ReadAll(io.LimitReader(input, summaryObjectLimit+1))
	if e != nil {
		return e
	}
	if len(raw) > summaryObjectLimit {
		return fmt.Errorf("summary object exceeds bounded object size")
	}
	if hashed {
		hash := sha256.Sum256(raw)
		if !strings.HasSuffix(key, "/"+hex.EncodeToString(hash[:])+".json.gz") {
			return fmt.Errorf("summary object hash mismatch")
		}
	}
	if e = json.Unmarshal(raw, out); e != nil {
		return e
	}
	return validateSummaryObject(out)
}
func (s *Storage) putSummary(ctx context.Context, tenant string, value any) (string, error) {
	raw, e := json.Marshal(value)
	if e != nil {
		return "", e
	}
	if len(raw) > summaryObjectLimit {
		return "", fmt.Errorf("summary object exceeds bounded object size")
	}
	hash := sha256.Sum256(raw)
	key := tenant + "/summaries/v1/" + s.summaryNamespace + "objects/" + hex.EncodeToString(hash[:]) + ".json.gz"
	var body bytes.Buffer
	z := gzip.NewWriter(&body)
	if _, e = z.Write(raw); e != nil {
		return "", e
	}
	if e = z.Close(); e != nil {
		return "", e
	}
	e = s.put(ctx, key, &body, int64(body.Len()), "application/gzip")
	if minio.ToErrorResponse(e).StatusCode == 412 {
		return key, nil
	}
	return key, e
}
func (s *Storage) contributions(ctx context.Context, tenant string, rows []rawObservation) (string, error) {
	return s.putSummary(ctx, tenant, makeContributions(rows))
}
func makeContributions(rows []rawObservation) chunkContributions {
	out := chunkContributions{}
	for _, r := range rows {
		key := minuteKey(r.At)
		block, minute := strconv.FormatInt(key/256, 10), strconv.FormatInt(key, 10)
		if out[r.Channel] == nil {
			out[r.Channel] = map[string]map[string]summaryTotals{}
		}
		if out[r.Channel][block] == nil {
			out[r.Channel][block] = map[string]summaryTotals{}
		}
		v := out[r.Channel][block][minute]
		v.add(r, true)
		out[r.Channel][block][minute] = v
	}
	return out
}
func (b *treeBuilder) node(ctx context.Context, ref summaryRef) (*summaryNode, error) {
	if ref.Object == "" {
		return &summaryNode{Version: 1}, nil
	}
	if n := b.nodes[ref.Object]; n != nil {
		return n, nil
	}
	n := &summaryNode{}
	if e := b.storage.summaryObject(ctx, ref.Object, n, true); e != nil {
		return nil, e
	}
	if n.Version != 1 || digest(n.Total) != digest(ref.Total) {
		return nil, fmt.Errorf("invalid summary node")
	}
	b.nodes[ref.Object] = n
	return n, nil
}
func (b *treeBuilder) save(ctx context.Context, n *summaryNode) (summaryRef, error) {
	key, e := b.storage.putSummary(ctx, b.tenant, n)
	if e != nil {
		return summaryRef{}, e
	}
	b.writes++
	b.nodes[key] = n
	return summaryRef{key, n.Total}, nil
}
func (b *treeBuilder) update(ctx context.Context, ref summaryRef, depth int, blocks map[int64]map[string]leafContribution) (summaryRef, error) {
	n, e := b.node(ctx, ref)
	if e != nil {
		return ref, e
	}
	// A loaded object is never mutated: other versions and queries may share it.
	next := &summaryNode{Version: 1}
	if depth == 0 {
		next.Inputs = map[string]leafContribution{}
		for k, v := range n.Inputs {
			next.Inputs[k] = v
		}
		for _, changes := range blocks {
			for k, v := range changes {
				next.Inputs[k] = v
			}
		}
		next.Minutes = map[string]summaryTotals{}
		keys := make([]string, 0, len(next.Inputs))
		for k := range next.Inputs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := next.Inputs[k]
			for minute, total := range v.Minutes {
				if !v.Active {
					total = inactiveContribution(total)
				}
				merged := next.Minutes[minute]
				merged.merge(total)
				next.Minutes[minute] = merged
			}
		}
		for _, v := range next.Minutes {
			next.Total.merge(v)
		}
	} else {
		next.Children = map[string]summaryRef{}
		for k, v := range n.Children {
			next.Children[k] = v
		}
		groups := map[string]map[int64]map[string]leafContribution{}
		for block, changes := range blocks {
			digit := strconv.FormatInt((block>>((depth-1)*8))&255, 10)
			if groups[digit] == nil {
				groups[digit] = map[int64]map[string]leafContribution{}
			}
			groups[digit][block] = changes
		}
		for digit, changes := range groups {
			child, e := b.update(ctx, next.Children[digit], depth-1, changes)
			if e != nil {
				return ref, e
			}
			next.Children[digit] = child
		}
		for _, v := range next.Children {
			next.Total.merge(v.Total)
		}
	}
	return b.save(ctx, next)
}
func (s *Service) readSummaryRoot(ctx context.Context, tenant, channel, revision string) (summaryRoot, error) {
	out := summaryRoot{Version: 1, ContentRevision: revision}
	if revision == "0" {
		return out, nil
	}
	key := summaryRootKey(tenant, channel, revision)
	e := s.db(ctx, func(tx pgx.Tx) error {
		var replacement string
		e := tx.QueryRow(ctx, `SELECT root_object FROM monitoring_summary_replacements WHERE tenant_id=$1 AND channel_id=$2 AND content_revision=$3 ORDER BY integrity_revision DESC LIMIT 1`, tenant, channel, revision).Scan(&replacement)
		if e == pgx.ErrNoRows {
			return nil
		}
		if e == nil {
			key = replacement
		}
		return e
	})
	if e != nil {
		return out, e
	}
	e = s.storage.summaryObject(ctx, key, &out, false)
	if minio.ToErrorResponse(e).Code == "NoSuchKey" {
		return out, errSummaryPending
	}
	if e == nil && (out.Version != 1 || out.ContentRevision != revision) {
		e = fmt.Errorf("summary root revision mismatch")
	}
	return out, e
}

// buildSummary applies only publications/revocations after the previous head.
// Each change contributes its actual adopted minutes, not its entire bounding
// interval. Rebuilding a sparse backfill therefore does not enumerate gaps.
func (s *Service) buildSummary(ctx context.Context, tenant, channel, revision, revoke string, watermark int64) (summaryRoot, error) {
	if root, e := s.readSummaryRoot(ctx, tenant, channel, revision); e == nil {
		return root, s.installSummaryHead(ctx, tenant, channel, revision, revoke, watermark, root.Root.Object)
	} else if !errors.Is(e, errSummaryPending) {
		return root, e
	}
	root := summaryRoot{Version: 1, ContentRevision: revision}
	var base int64
	var changes []summaryChange
	e := s.db(ctx, func(tx pgx.Tx) error {
		var key string
		e := tx.QueryRow(ctx, `SELECT root_object,watermark FROM monitoring_summary_heads WHERE tenant_id=$1 AND channel_id=$2 AND level='tree-v1' AND watermark<=$3`, tenant, channel, watermark).Scan(&key, &base)
		if e != nil && e != pgx.ErrNoRows {
			return e
		}
		if e == nil {
			root.Root.Object = key
		}
		rows, e := tx.Query(ctx, `SELECT c.id::text,COALESCE((SELECT cr.object_key FROM monitoring_contribution_replacements cr WHERE cr.tenant_id=c.tenant_id AND cr.chunk_id=c.id ORDER BY cr.created_at DESC,cr.job_id DESC LIMIT 1),c.summary_object),(p.revoke_sequence IS NULL OR p.revoke_sequence>$3) FROM monitoring_publication_chunks pc JOIN monitoring_chunks c ON c.tenant_id=pc.tenant_id AND c.id=pc.chunk_id JOIN monitoring_publications p ON p.tenant_id=pc.tenant_id AND p.id=pc.publication_id JOIN monitoring_import_batches b ON b.tenant_id=p.tenant_id AND b.id=p.batch_id WHERE p.tenant_id=$1 AND b.semantic_snapshot->'actual_coverage' ? $2 AND p.sequence<=$3 AND (p.sequence>$4 OR p.revoke_sequence>$4 AND p.revoke_sequence<=$3) ORDER BY p.sequence,c.id`, tenant, channel, watermark, base)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var v summaryChange
			if e = rows.Scan(&v.Chunk, &v.Object, &v.Active); e != nil {
				return e
			}
			changes = append(changes, v)
		}
		return rows.Err()
	})
	if e != nil {
		return root, e
	}
	if root.Root.Object != "" {
		var prior summaryNode
		if e = s.storage.summaryObject(ctx, root.Root.Object, &prior, true); e != nil {
			return root, e
		}
		root.Root.Total = prior.Total
	}
	b := treeBuilder{s.storage, tenant, map[string]*summaryNode{}, 0}
	for _, change := range changes {
		if e = ctx.Err(); e != nil {
			return root, e
		}
		var contribution chunkContributions
		if e = s.storage.summaryObject(ctx, change.Object, &contribution, true); e != nil {
			return root, e
		}
		blocks := map[int64]map[string]leafContribution{}
		for block, minutes := range contribution[channel] {
			id, e := strconv.ParseInt(block, 10, 64)
			if e != nil || id < 0 {
				return root, fmt.Errorf("invalid contribution block")
			}
			blocks[id] = map[string]leafContribution{change.Chunk: {Active: change.Active, Minutes: minutes}}
		}
		if len(blocks) > 0 {
			root.Root, e = b.update(ctx, root.Root, 3, blocks)
			if e != nil {
				return root, e
			}
		}
		// Bound the worker's cache; immutable child objects can be reread safely.
		b.nodes = map[string]*summaryNode{}
	}
	key := summaryRootKey(tenant, channel, revision)
	raw := encodeJSON(root)
	if e = s.storage.put(ctx, key, bytes.NewReader(raw), int64(len(raw)), "application/json"); e != nil && minio.ToErrorResponse(e).StatusCode != 412 {
		return root, e
	}
	// Head installation is a short CAS. An older worker cannot roll it back.
	return root, s.installSummaryHead(ctx, tenant, channel, revision, revoke, watermark, root.Root.Object)
}
func (s *Service) installSummaryHead(ctx context.Context, tenant, channel, revision, revoke string, watermark int64, object string) error {
	if revision == "0" {
		return nil
	}
	return s.db(ctx, func(tx pgx.Tx) error {
		_, e := tx.Exec(ctx, `INSERT INTO monitoring_summary_heads(tenant_id,channel_id,level,revision,watermark,revoke_revision,root_object) VALUES($1,$2,'tree-v1',$3,$4,$5,$6) ON CONFLICT(tenant_id,channel_id,level) DO UPDATE SET revision=EXCLUDED.revision,watermark=EXCLUDED.watermark,revoke_revision=EXCLUDED.revoke_revision,root_object=EXCLUDED.root_object WHERE monitoring_summary_heads.watermark<EXCLUDED.watermark`, tenant, channel, revision, watermark, revoke, object)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `DELETE FROM monitoring_dirty_ranges WHERE tenant_id=$1 AND channel_id=$2 AND target_sequence<=$3`, tenant, channel, watermark)
		return e
	})
}

type summaryReader struct {
	service        *Service
	plan           queryPlan
	tenant         string
	budget         int
	rawBudget      int
	nodes          map[string]*summaryNode
	history        []selectedChunk
	historyLoaded  bool
	reads, rawRows int
	cacheBytes     int
}

func (r *summaryReader) node(ctx context.Context, ref summaryRef) (*summaryNode, error) {
	if ref.Object == "" {
		return &summaryNode{Version: 1}, nil
	}
	if n := r.nodes[ref.Object]; n != nil {
		return n, nil
	}
	if r.budget > 0 && r.reads >= r.budget {
		return nil, errQueryBudget
	}
	r.reads++
	n := &summaryNode{}
	if e := r.service.storage.summaryObject(ctx, ref.Object, n, true); e != nil {
		return nil, e
	}
	if n.Version != 1 || digest(n.Total) != digest(ref.Total) {
		return nil, fmt.Errorf("invalid summary node")
	}
	size := len(encodeJSON(n))
	if r.cacheBytes+size > 16<<20 || len(r.nodes) >= 2048 {
		r.nodes = map[string]*summaryNode{}
		r.cacheBytes = 0
	}
	if size <= 16<<20 {
		r.nodes[ref.Object] = n
		r.cacheBytes += size
	}
	return n, nil
}
func (r *summaryReader) collect(ctx context.Context, channel string, ref summaryRef, query *Window) (summaryTotals, error) {
	if !ref.Total.overlaps(r.plan.windows[channel], query) {
		return summaryTotals{}, nil
	}
	if ref.Total.covered(r.plan.windows[channel], query) {
		return ref.Total, nil
	}
	n, e := r.node(ctx, ref)
	if e != nil {
		return summaryTotals{}, e
	}
	out := summaryTotals{}
	for _, child := range n.Children {
		v, e := r.collect(ctx, channel, child, query)
		if e != nil {
			return out, e
		}
		out.merge(v)
	}
	for minute, v := range n.Minutes {
		if !v.overlaps(r.plan.windows[channel], query) {
			continue
		}
		if v.covered(r.plan.windows[channel], query) {
			out.merge(v)
			continue
		}
		key, e := strconv.ParseInt(minute, 10, 64)
		if e != nil {
			return out, e
		}
		w := minuteWindow(key)
		if query != nil {
			var ok bool
			w, ok = w.Intersect(*query)
			if !ok {
				continue
			}
		}
		v, e = r.raw(ctx, channel, w)
		if e != nil {
			return out, e
		}
		out.merge(v)
	}
	return out, nil
}
func (r *summaryReader) raw(ctx context.Context, channel string, window Window) (summaryTotals, error) {
	out := summaryTotals{}
	if !r.historyLoaded {
		watermark, _ := revisionNumber(r.plan.snapshot.Watermark)
		e := r.service.db(ctx, func(tx pgx.Tx) error {
			var e error
			r.history, e = manifestTx(ctx, tx, r.tenant, r.plan.windows, watermark, "history", mustRevision(r.plan.snapshot.RouteRevision))
			return e
		})
		if e != nil {
			return out, e
		}
		r.historyLoaded = true
	}
	revoked := map[string]bool{}
	for _, c := range r.history {
		revoked[c.Batch] = c.Revoked
	}
	e := r.service.storage.read(ctx, rawRead{Manifest: r.history, Tenant: r.tenant, Windows: map[string][]Window{channel: r.plan.windows[channel]}, Range: &window}, func(row rawObservation) error {
		r.rawRows++
		if r.rawBudget > 0 && r.rawRows > r.rawBudget {
			return errQueryBudget
		}
		out.add(row, !revoked[row.Batch])
		return nil
	})
	return out, e
}
