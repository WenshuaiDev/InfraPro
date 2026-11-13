package resultstore

import (
	"context"
	"fmt"
)

type verifiedRangeTree struct {
	first    int64
	last     *int64
	count    uint64
	priority int64
}

// RangeVerifier shares a bounded subtree verification cache across historical
// roots. Persistent trees reuse most subtrees, so historical reconciliation
// need not repeatedly scan every observation range of every prior generation.
type RangeVerifier struct {
	tree     rangeTree
	consume  func(RangeEntry) error
	verified map[string]verifiedRangeTree
	order    []string
	next     int
}

func (s *Store) RangeVerifier(tenant string, consume func(RangeEntry) error) *RangeVerifier {
	return &RangeVerifier{tree: rangeTree{s, tenant, map[string]rangeNode{}}, consume: consume, verified: map[string]verifiedRangeTree{}}
}
func (v *RangeVerifier) Verify(ctx context.Context, root string) (uint64, error) {
	r, e := v.walk(ctx, root, 0)
	return r.count, e
}
func (v *RangeVerifier) walk(ctx context.Context, root string, depth int) (verifiedRangeTree, error) {
	if root == "" {
		return verifiedRangeTree{}, nil
	}
	if e := ctx.Err(); e != nil {
		return verifiedRangeTree{}, e
	}
	if depth > 128 {
		return verifiedRangeTree{}, fmt.Errorf("range_tree_depth_exceeded")
	}
	if found, ok := v.verified[root]; ok {
		return found, nil
	}
	n, e := v.tree.read(ctx, root)
	if e != nil {
		return verifiedRangeTree{}, e
	}
	left, e := v.walk(ctx, n.Left, depth+1)
	if e != nil {
		return left, e
	}
	right, e := v.walk(ctx, n.Right, depth+1)
	if e != nil {
		return right, e
	}
	if left.count > 0 && (left.last == nil || *left.last > n.Entry.From || earlierPriority(left.priority, n.Entry.From)) {
		return verifiedRangeTree{}, fmt.Errorf("range_tree_order_mismatch")
	}
	if right.count > 0 && (n.Entry.To == nil || right.first < *n.Entry.To || earlierPriority(right.priority, n.Entry.From)) {
		return verifiedRangeTree{}, fmt.Errorf("range_tree_order_mismatch")
	}
	if e = v.consume(n.Entry); e != nil {
		return verifiedRangeTree{}, e
	}
	out := verifiedRangeTree{first: n.Entry.From, last: n.Entry.To, count: left.count + 1 + right.count, priority: n.Entry.From}
	if left.count > 0 {
		out.first = left.first
	}
	if right.count > 0 {
		out.last = right.last
	}
	if len(v.order) < 4096 {
		v.order = append(v.order, root)
	} else {
		delete(v.verified, v.order[v.next])
		v.order[v.next] = root
		v.next = (v.next + 1) % len(v.order)
	}
	v.verified[root] = out
	return out, nil
}
