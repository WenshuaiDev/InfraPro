package compute

import (
	"context"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
	"math/big"
)

// A deterministic order-statistic treap retains duplicates and supports exact
// rank selection. Priority depends only on the immutable sample ordinal; no
// process RNG or arrival timing influences quantiles or extrema ties.
type orderNode struct {
	sample       Sample
	ordinal      int
	priority     uint64
	left, right  *orderNode
	size         int
	sum, squares *monitoringvalue.Number
}
type orderTree struct {
	root               *orderNode
	numeric, aggregate bool
}

func priority(i int) uint64 {
	x := uint64(i) + 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}
func size(n *orderNode) int {
	if n == nil {
		return 0
	}
	return n.size
}
func (t *orderTree) refresh(n *orderNode) error {
	n.size = 1 + size(n.left) + size(n.right)
	if !t.aggregate {
		return nil
	}
	n.sum = n.sample.Value
	var e error
	n.squares, e = monitoringvalue.Binary("multiply", n.sample.Value, n.sample.Value)
	if e != nil {
		return e
	}
	for _, child := range []*orderNode{n.left, n.right} {
		if child != nil {
			n.sum, e = monitoringvalue.Binary("add", n.sum, child.sum)
			if e != nil {
				return e
			}
			n.squares, e = monitoringvalue.Binary("add", n.squares, child.squares)
			if e != nil {
				return e
			}
		}
	}
	return nil
}
func (t *orderTree) compare(ctx context.Context, a Sample, i int, b *orderNode) (int, error) {
	if t.numeric {
		c, e := a.Value.Compare(ctx, b.sample.Value)
		if e != nil || c != 0 {
			return c, e
		}
		if a.At < b.sample.At {
			return -1, nil
		}
		if a.At > b.sample.At {
			return 1, nil
		}
	}
	if i < b.ordinal {
		return -1, nil
	}
	if i > b.ordinal {
		return 1, nil
	}
	return 0, nil
}
func (t *orderTree) insert(ctx context.Context, n *orderNode, s Sample, i int) (*orderNode, error) {
	if n == nil {
		n = &orderNode{sample: s, ordinal: i, priority: priority(i)}
		return n, t.refresh(n)
	}
	c, e := t.compare(ctx, s, i, n)
	if e != nil {
		return n, e
	}
	if c < 0 {
		n.left, e = t.insert(ctx, n.left, s, i)
		if e == nil && n.left.priority < n.priority {
			x := n.left
			n.left = x.right
			x.right = n
			if e = t.refresh(n); e != nil {
				return n, e
			}
			n = x
		}
	} else {
		n.right, e = t.insert(ctx, n.right, s, i)
		if e == nil && n.right.priority < n.priority {
			x := n.right
			n.right = x.left
			x.left = n
			if e = t.refresh(n); e != nil {
				return n, e
			}
			n = x
		}
	}
	if e != nil {
		return n, e
	}
	return n, t.refresh(n)
}
func (t *orderTree) merge(a, b *orderNode) (*orderNode, error) {
	if a == nil {
		return b, nil
	}
	if b == nil {
		return a, nil
	}
	var e error
	if a.priority < b.priority {
		a.right, e = t.merge(a.right, b)
		if e == nil {
			e = t.refresh(a)
		}
		return a, e
	}
	b.left, e = t.merge(a, b.left)
	if e == nil {
		e = t.refresh(b)
	}
	return b, e
}
func (t *orderTree) remove(ctx context.Context, n *orderNode, s Sample, i int) (*orderNode, error) {
	if n == nil {
		return nil, nil
	}
	c, e := t.compare(ctx, s, i, n)
	if e != nil {
		return n, e
	}
	if c == 0 {
		return t.merge(n.left, n.right)
	}
	if c < 0 {
		n.left, e = t.remove(ctx, n.left, s, i)
	} else {
		n.right, e = t.remove(ctx, n.right, s, i)
	}
	if e == nil {
		e = t.refresh(n)
	}
	return n, e
}
func (t *orderTree) add(ctx context.Context, s Sample, i int) error {
	var e error
	t.root, e = t.insert(ctx, t.root, s, i)
	return e
}
func (t *orderTree) delete(ctx context.Context, s Sample, i int) error {
	var e error
	t.root, e = t.remove(ctx, t.root, s, i)
	return e
}
func (t *orderTree) rank(k int) *Sample {
	n := t.root
	for n != nil {
		l := size(n.left)
		if k == l {
			return &n.sample
		}
		if k < l {
			n = n.left
		} else {
			k -= l + 1
			n = n.right
		}
	}
	return nil
}
func (t *orderTree) quantile(p *big.Rat) (*monitoringvalue.Number, error) {
	h := new(big.Rat).Mul(big.NewRat(int64(size(t.root)-1), 100), p)
	k := new(big.Int).Quo(h.Num(), h.Denom())
	fraction := new(big.Rat).Sub(h, new(big.Rat).SetInt(k))
	a := t.rank(int(k.Int64())).Value
	if fraction.Sign() == 0 {
		return a, nil
	}
	b := t.rank(int(k.Int64()) + 1).Value
	delta, e := monitoringvalue.Binary("subtract", b, a)
	if e != nil {
		return nil, e
	}
	f, e := monitoringvalue.Rational(fraction)
	if e != nil {
		return nil, e
	}
	v, e := monitoringvalue.Binary("multiply", delta, f)
	if e != nil {
		return nil, e
	}
	return monitoringvalue.Binary("add", a, v)
}

func (t *orderTree) boundary(ctx context.Context, value *monitoringvalue.Number, inclusive bool) (int, error) {
	count := 0
	n := t.root
	for n != nil {
		cmp, e := n.sample.Value.Compare(ctx, value)
		if e != nil {
			return 0, e
		}
		if cmp < 0 || inclusive && cmp == 0 {
			count += size(n.left) + 1
			n = n.right
		} else {
			n = n.left
		}
	}
	return count, nil
}
func (t *orderTree) extrema(ctx context.Context) (minAt, maxAt int64, minTies, maxTies uint64, err error) {
	minimum, maximum := t.rank(0), t.rank(size(t.root)-1)
	lowEnd, e := t.boundary(ctx, minimum.Value, true)
	if e != nil {
		return 0, 0, 0, 0, e
	}
	highStart, e := t.boundary(ctx, maximum.Value, false)
	if e != nil {
		return 0, 0, 0, 0, e
	}
	return minimum.At, t.rank(highStart).At, uint64(lowEnd), uint64(size(t.root) - highStart), nil
}
