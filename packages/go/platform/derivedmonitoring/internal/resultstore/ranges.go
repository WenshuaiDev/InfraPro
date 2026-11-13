package resultstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/derivedmonitoring/internal/planning"
	"github.com/google/uuid"
)

type RangeEntry struct {
	From           int64  `json:"from_ns,string"`
	To             *int64 `json:"to_ns,string"`
	Generation     string `json:"generation_id"`
	Manifest       string `json:"manifest_root"`
	SourcePosition string `json:"source_position"`
}

// WalkRanges validates a complete immutable index with bounded memory. It is
// used by recovery, where truncating to an interactive page would hide damage.
func (s *Store) WalkRanges(ctx context.Context, tenant, root string, consume func(RangeEntry) error) error {
	tree := rangeTree{s, tenant, map[string]rangeNode{}}
	var previous *RangeEntry
	var walk func(string, int) error
	walk = func(hash string, depth int) error {
		if hash == "" {
			return nil
		}
		if e := ctx.Err(); e != nil {
			return e
		}
		if depth > 128 {
			return fmt.Errorf("range_tree_depth_exceeded")
		}
		n, e := tree.read(ctx, hash)
		if e != nil {
			return e
		}
		if e = walk(n.Left, depth+1); e != nil {
			return e
		}
		if previous != nil && (previous.To == nil || *previous.To > n.Entry.From) {
			return fmt.Errorf("range_tree_order_mismatch")
		}
		v := n.Entry
		previous = &v
		if e = consume(v); e != nil {
			return e
		}
		return walk(n.Right, depth+1)
	}
	return walk(root, 0)
}

func (e RangeEntry) Interval() planning.Range { return planning.Range{From: e.From, To: e.To} }
func (e RangeEntry) valid() bool {
	if !e.Interval().Valid() || len(e.Manifest) != 64 {
		return false
	}
	if _, err := hex.DecodeString(e.Manifest); err != nil {
		return false
	}
	if id, err := uuid.Parse(e.Generation); err != nil || id.String() != e.Generation {
		return false
	}
	n, err := strconv.ParseInt(e.SourcePosition, 10, 64)
	return err == nil && n >= 0 && strconv.FormatInt(n, 10) == e.SourcePosition
}

type rangeNode struct {
	Schema string     `json:"schema"`
	Entry  RangeEntry `json:"entry"`
	Left   string     `json:"left"`
	Right  string     `json:"right"`
}

// The immutable interval treap gives range replacement a logarithmic path.
// Split nodes retain their generation; an empty generation still owns its
// entire replacement interval so a previous value cannot reappear there.
type rangeTree struct {
	store  *Store
	tenant string
	cache  map[string]rangeNode
}

func priority(from int64) [32]byte { return sha256.Sum256([]byte(strconv.FormatInt(from, 10))) }
func earlierPriority(a, b int64) bool {
	pa, pb := priority(a), priority(b)
	for i := range pa {
		if pa[i] != pb[i] {
			return pa[i] < pb[i]
		}
	}
	return a < b
}
func (t *rangeTree) read(ctx context.Context, hash string) (rangeNode, error) {
	if n, ok := t.cache[hash]; ok {
		return n, nil
	}
	var n rangeNode
	if e := t.store.ReadDocument(ctx, t.tenant, "ranges", hash, &n); e != nil {
		return n, e
	}
	if n.Schema != "derived-range-node/1" || !n.Entry.valid() || n.Left != "" && len(n.Left) != 64 || n.Right != "" && len(n.Right) != 64 {
		return n, fmt.Errorf("invalid_range_node")
	}
	if len(t.cache) < 1024 {
		t.cache[hash] = n
	}
	return n, nil
}
func (t *rangeTree) write(ctx context.Context, n rangeNode) (string, error) {
	n.Schema = "derived-range-node/1"
	if !n.Entry.valid() {
		return "", fmt.Errorf("invalid_range_node")
	}
	hash, e := t.store.SealDocument(ctx, t.tenant, "ranges", n)
	if e == nil && len(t.cache) < 1024 {
		t.cache[hash] = n
	}
	return hash, e
}
func (t *rangeTree) merge(ctx context.Context, a, b string, depth int) (string, error) {
	if e := ctx.Err(); e != nil {
		return "", e
	}
	if depth > 128 {
		return "", fmt.Errorf("range_tree_depth_exceeded")
	}
	if a == "" {
		return b, nil
	}
	if b == "" {
		return a, nil
	}
	left, e := t.read(ctx, a)
	if e != nil {
		return "", e
	}
	right, e := t.read(ctx, b)
	if e != nil {
		return "", e
	}
	if earlierPriority(left.Entry.From, right.Entry.From) {
		left.Right, e = t.merge(ctx, left.Right, b, depth+1)
		if e != nil {
			return "", e
		}
		return t.write(ctx, left)
	}
	right.Left, e = t.merge(ctx, a, right.Left, depth+1)
	if e != nil {
		return "", e
	}
	return t.write(ctx, right)
}
func (t *rangeTree) split(ctx context.Context, root string, at int64, depth int) (string, string, error) {
	if root == "" {
		return "", "", nil
	}
	if e := ctx.Err(); e != nil {
		return "", "", e
	}
	if depth > 128 {
		return "", "", fmt.Errorf("range_tree_depth_exceeded")
	}
	n, e := t.read(ctx, root)
	if e != nil {
		return "", "", e
	}
	if at <= n.Entry.From {
		a, b, e := t.split(ctx, n.Left, at, depth+1)
		if e != nil {
			return "", "", e
		}
		n.Left = b
		right, e := t.write(ctx, n)
		return a, right, e
	}
	if n.Entry.To != nil && at >= *n.Entry.To {
		a, b, e := t.split(ctx, n.Right, at, depth+1)
		if e != nil {
			return "", "", e
		}
		n.Right = a
		left, e := t.write(ctx, n)
		return left, b, e
	}
	// The split crosses one interval. Reinsert the new right fragment because
	// its deterministic priority follows its new start boundary.
	left, right := n.Entry, n.Entry
	left.To = planning.End(at)
	right.From = at
	l, e := t.write(ctx, rangeNode{Entry: left})
	if e != nil {
		return "", "", e
	}
	r, e := t.write(ctx, rangeNode{Entry: right})
	if e != nil {
		return "", "", e
	}
	l, e = t.merge(ctx, n.Left, l, depth+1)
	if e != nil {
		return "", "", e
	}
	r, e = t.merge(ctx, r, n.Right, depth+1)
	return l, r, e
}
func (s *Store) ReplaceRange(ctx context.Context, tenant, root string, entry RangeEntry) (string, error) {
	if !entry.valid() {
		return "", fmt.Errorf("invalid_range_entry")
	}
	tree := rangeTree{s, tenant, map[string]rangeNode{}}
	left, rest, e := tree.split(ctx, root, entry.From, 0)
	if e != nil {
		return "", e
	}
	right := ""
	if entry.To != nil {
		_, right, e = tree.split(ctx, rest, *entry.To, 0)
		if e != nil {
			return "", e
		}
	}
	middle, e := tree.write(ctx, rangeNode{Entry: entry})
	if e != nil {
		return "", e
	}
	joined, e := tree.merge(ctx, left, middle, 0)
	if e != nil {
		return "", e
	}
	return tree.merge(ctx, joined, right, 0)
}
func (s *Store) ReadRanges(ctx context.Context, tenant, root string, wanted planning.Range, limit int) ([]RangeEntry, error) {
	if !wanted.Valid() || limit < 1 {
		return nil, fmt.Errorf("invalid_range_query")
	}
	tree := rangeTree{s, tenant, map[string]rangeNode{}}
	out := []RangeEntry{}
	var walk func(string, int) error
	walk = func(root string, depth int) error {
		if root == "" {
			return nil
		}
		if e := ctx.Err(); e != nil {
			return e
		}
		if depth > 128 {
			return fmt.Errorf("range_tree_depth_exceeded")
		}
		n, e := tree.read(ctx, root)
		if e != nil {
			return e
		}
		if wanted.From < n.Entry.From {
			if e = walk(n.Left, depth+1); e != nil {
				return e
			}
		}
		if planning.Overlap(wanted, n.Entry.Interval()) {
			if len(out) >= limit {
				return fmt.Errorf("range_query_budget_exceeded")
			}
			if len(out) > 0 {
				previous := out[len(out)-1]
				if previous.To == nil || *previous.To > n.Entry.From {
					return fmt.Errorf("range_tree_order_mismatch")
				}
			}
			out = append(out, n.Entry)
		}
		if wanted.To == nil || n.Entry.To != nil && *n.Entry.To < *wanted.To {
			if e = walk(n.Right, depth+1); e != nil {
				return e
			}
		}
		return nil
	}
	if e := walk(root, 0); e != nil {
		return nil, e
	}
	return out, nil
}
