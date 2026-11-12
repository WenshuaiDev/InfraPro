package monitoring

import (
	"context"
	"sort"
	"strings"
	"sync"
)

// read merges one ordered stream per physical shard. Only a single row per
// shard waits in Go; cancellation closes all ClickHouse scans before returning.
func (s *Storage) read(ctx context.Context, q rawRead, consume func(rawObservation) error) error {
	if q.Manifest == nil {
		return s.readOne(ctx, q, consume)
	}
	groups := map[string][]selectedChunk{}
	for _, c := range q.Manifest {
		name := c.Physical
		if name == "" {
			name = "local"
		}
		groups[name] = append(groups[name], c)
	}
	if len(groups) == 0 {
		return nil
	}
	if len(groups) == 1 {
		for name, entries := range groups {
			p, e := s.physical(name)
			if e != nil {
				return e
			}
			q.Manifest = entries
			return p.readOne(ctx, q, consume)
		}
	}
	work, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	defer func() { cancel(); wg.Wait() }()
	type item struct {
		row rawObservation
		err error
		end bool
	}
	type stream struct {
		ch   chan item
		head item
	}
	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)
	streams := make([]*stream, 0, len(names))
	for _, name := range names {
		p, e := s.physical(name)
		if e != nil {
			return e
		}
		sub := q
		sub.Manifest = groups[name]
		st := &stream{ch: make(chan item)}
		streams = append(streams, st)
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer close(st.ch)
			err := p.readOne(work, sub, func(r rawObservation) error {
				select {
				case st.ch <- item{row: r}:
					return nil
				case <-work.Done():
					return work.Err()
				}
			})
			select {
			case st.ch <- item{end: true, err: err}:
			case <-work.Done():
			}
		}()
	}
	next := func(st *stream) error {
		select {
		case v, ok := <-st.ch:
			if !ok {
				st.head = item{end: true}
			} else {
				st.head = v
			}
			return st.head.err
		case <-work.Done():
			return work.Err()
		}
	}
	for _, st := range streams {
		if e := next(st); e != nil {
			return e
		}
	}
	for n := 0; q.Limit <= 0 || n < q.Limit; n++ {
		var best *stream
		for _, st := range streams {
			if st.head.end {
				continue
			}
			if best == nil {
				best = st
				continue
			}
			cmp := compareRead(st.head.row, best.head.row, q.Order)
			if q.Reverse {
				cmp = -cmp
			}
			if cmp < 0 {
				best = st
			}
		}
		if best == nil {
			return nil
		}
		if e := consume(best.head.row); e != nil {
			return e
		}
		if e := next(best); e != nil {
			return e
		}
	}
	return nil
}
func compareRead(a, b rawObservation, order string) int {
	if order == "channel" {
		if c := strings.Compare(a.Channel, b.Channel); c != 0 {
			return c
		}
	}
	if a.At < b.At {
		return -1
	}
	if a.At > b.At {
		return 1
	}
	for _, pair := range [][2]string{{a.Channel, b.Channel}, {a.Observation, b.Observation}, {a.Adoption, b.Adoption}} {
		if c := strings.Compare(pair[0], pair[1]); c != 0 {
			return c
		}
	}
	return 0
}
