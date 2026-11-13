// Package exactsort performs bounded external sorting of exact rational values.
// Its caller owns durable segment records and immutable source qualification.
package exactsort

import (
	"bufio"
	"container/heap"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"syscall"

	"github.com/WenshuaiDev/InfraPro/packages/go/platform/monitoringvalue"
)

type Segment struct {
	SHA256 string `json:"sha256"`
	Rows   uint64 `json:"rows,string"`
	Bytes  uint64 `json:"bytes,string"`
}
type Options struct {
	Directory   string
	MemoryBytes uint64
	Budget      *Budget
	// Seal must finish durably before a segment can participate in a merge.
	Seal      func(context.Context, string, Segment) error
	ChunkRows int
	FanIn     int
}
type Budget struct{ Limit, Used, Peak uint64 }

func temporaryError(e error) error {
	if errors.Is(e, syscall.ENOSPC) || errors.Is(e, syscall.EDQUOT) {
		return fmt.Errorf("temporary_budget_exceeded: %w", e)
	}
	return e
}

func (b *Budget) reserve(n uint64) error {
	if b == nil || b.Used > b.Limit || n > b.Limit-b.Used {
		return fmt.Errorf("temporary_budget_exceeded")
	}
	b.Used += n
	b.Peak = max(b.Peak, b.Used)
	return nil
}

type file struct {
	path    string
	segment Segment
}
type Sorter struct {
	options   Options
	directory string
	values    []*big.Rat
	memory    uint64
	files     []file
	sequence  uint64
	rows      uint64
	closed    bool
	lock      *os.File
}

func New(options Options) (*Sorter, error) {
	if options.MemoryBytes < 16<<10 || options.Budget == nil || options.Budget.Limit < 16<<10 {
		return nil, fmt.Errorf("external_sort_budget_exceeded")
	}
	if options.ChunkRows == 0 {
		options.ChunkRows = 16384
	}
	if options.ChunkRows < 1 || options.ChunkRows > 100000 {
		return nil, fmt.Errorf("invalid_sort_chunk")
	}
	if options.FanIn == 0 {
		options.FanIn = 32
	}
	if options.FanIn < 2 || options.FanIn > 64 {
		return nil, fmt.Errorf("invalid_sort_fan_in")
	}
	dir, e := os.MkdirTemp(options.Directory, "infrapro-exact-sort-")
	if e != nil {
		return nil, temporaryError(e)
	}
	lock, e := os.OpenFile(filepath.Join(dir, ".active"), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if e != nil {
		os.RemoveAll(dir)
		return nil, temporaryError(e)
	}
	if e = lockFile(lock); e != nil {
		lock.Close()
		os.RemoveAll(dir)
		return nil, e
	}
	return &Sorter{options: options, directory: dir, lock: lock}, nil
}
func (s *Sorter) Add(ctx context.Context, value *big.Rat) error {
	if s.closed {
		return fmt.Errorf("sorter_closed")
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	if value == nil {
		return fmt.Errorf("missing_sort_value")
	}
	if e := monitoringvalue.Check(value); e != nil {
		return e
	}
	size := uint64((value.Num().BitLen()+value.Denom().BitLen()+7)/8 + 160)
	if size > s.options.MemoryBytes {
		return fmt.Errorf("external_sort_memory_budget_exceeded")
	}
	if s.memory+size > s.options.MemoryBytes {
		if e := s.flush(ctx); e != nil {
			return e
		}
	}
	s.values = append(s.values, new(big.Rat).Set(value))
	s.memory += size
	s.rows++
	if len(s.values) >= s.options.ChunkRows {
		return s.flush(ctx)
	}
	return nil
}
func (s *Sorter) flush(ctx context.Context) error {
	if len(s.values) == 0 {
		return nil
	}
	// Values have bounded numerator and denominator sizes. No binary floating
	// point enters the comparator, run format, merge, or rank selection.
	var comparisons uint64
	var cancelled error
	slices.SortFunc(s.values, func(a, b *big.Rat) int {
		comparisons++
		if comparisons%256 == 0 {
			cancelled = ctx.Err()
		}
		if cancelled != nil {
			return 0
		}
		return a.Cmp(b)
	})
	if e := ctx.Err(); e != nil {
		return e
	}
	index := 0
	f, e := s.write(ctx, func() (*big.Rat, error) {
		if index == len(s.values) {
			return nil, io.EOF
		}
		r := s.values[index]
		index++
		return r, nil
	})
	if e != nil {
		return e
	}
	s.files = append(s.files, f)
	clear(s.values)
	s.values = nil
	s.memory = 0
	return nil
}

type quotaWriter struct {
	writer  io.Writer
	budget  *Budget
	written uint64
}

func (w *quotaWriter) Write(p []byte) (int, error) {
	if e := w.budget.reserve(uint64(len(p))); e != nil {
		return 0, e
	}
	n, e := w.writer.Write(p)
	w.budget.Used -= uint64(len(p) - n)
	w.written += uint64(n)
	return n, e
}
func (s *Sorter) write(ctx context.Context, next func() (*big.Rat, error)) (out file, err error) {
	defer func() { err = temporaryError(err) }()
	s.sequence++
	path := filepath.Join(s.directory, fmt.Sprintf("%020d.rat", s.sequence))
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return out, e
	}
	hash := sha256.New()
	quota := &quotaWriter{writer: io.MultiWriter(f, hash), budget: s.options.Budget}
	writer := bufio.NewWriterSize(quota, 64<<10)
	defer func() {
		f.Close()
		if err != nil {
			os.Remove(path)
			s.options.Budget.Used -= quota.written
		}
	}()
	var count uint64
	for {
		if count%256 == 0 {
			if e = ctx.Err(); e != nil {
				return out, e
			}
		}
		value, e := next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return out, e
		}
		if _, e = writer.WriteString(value.RatString() + "\n"); e != nil {
			return out, e
		}
		count++
	}
	if e = writer.Flush(); e != nil {
		return out, e
	}
	if e = f.Sync(); e != nil {
		return out, e
	}
	if e = f.Close(); e != nil {
		return out, e
	}
	out = file{path, Segment{SHA256: hex.EncodeToString(hash.Sum(nil)), Rows: count, Bytes: quota.written}}
	if s.options.Seal != nil {
		if e = s.options.Seal(ctx, path, out.segment); e != nil {
			return file{}, e
		}
	}
	return out, nil
}

type reader struct {
	file         *os.File
	scan         *bufio.Scanner
	segment      Segment
	hash         io.Writer
	count, bytes uint64
	previous     *big.Rat
}

func open(f file) (*reader, error) {
	in, e := os.Open(f.path)
	if e != nil {
		return nil, e
	}
	h := sha256.New()
	scan := bufio.NewScanner(io.TeeReader(in, h))
	scan.Buffer(make([]byte, 4096), 32<<10)
	return &reader{file: in, scan: scan, segment: f.segment, hash: h}, nil
}
func (r *reader) next() (*big.Rat, error) {
	if !r.scan.Scan() {
		if e := r.scan.Err(); e != nil {
			return nil, e
		}
		h := r.hash.(interface{ Sum([]byte) []byte })
		if r.count != r.segment.Rows || r.bytes != r.segment.Bytes || hex.EncodeToString(h.Sum(nil)) != r.segment.SHA256 {
			return nil, fmt.Errorf("sort_segment_integrity_unavailable")
		}
		return nil, io.EOF
	}
	line := r.scan.Text()
	r.bytes += uint64(len(line) + 1)
	r.count++
	value, ok := new(big.Rat).SetString(line)
	if !ok || value.RatString() != line || r.count > r.segment.Rows {
		return nil, fmt.Errorf("invalid_sort_segment")
	}
	if e := monitoringvalue.Check(value); e != nil {
		return nil, e
	}
	if r.previous != nil && r.previous.Cmp(value) > 0 {
		return nil, fmt.Errorf("unordered_sort_segment")
	}
	r.previous = value
	return value, nil
}

type entry struct {
	value *big.Rat
	index int
}
type mergeHeap []entry

func (h mergeHeap) Len() int { return len(h) }
func (h mergeHeap) Less(i, j int) bool {
	c := h[i].value.Cmp(h[j].value)
	return c < 0 || c == 0 && h[i].index < h[j].index
}
func (h mergeHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *mergeHeap) Push(v any)   { *h = append(*h, v.(entry)) }
func (h *mergeHeap) Pop() any     { old := *h; last := old[len(old)-1]; *h = old[:len(old)-1]; return last }

type merger struct {
	readers []*reader
	heap    mergeHeap
}

func newMerger(files []file) (m *merger, err error) {
	m = &merger{}
	defer func() {
		if err != nil {
			m.close()
		}
	}()
	for i, f := range files {
		r, e := open(f)
		if e != nil {
			return m, e
		}
		m.readers = append(m.readers, r)
		v, e := r.next()
		if e == io.EOF {
			continue
		}
		if e != nil {
			return m, e
		}
		heap.Push(&m.heap, entry{v, i})
	}
	return m, nil
}
func (m *merger) next() (*big.Rat, error) {
	if len(m.heap) == 0 {
		return nil, io.EOF
	}
	v := heap.Pop(&m.heap).(entry)
	next, e := m.readers[v.index].next()
	if e != nil && e != io.EOF {
		return nil, e
	}
	if e == nil {
		heap.Push(&m.heap, entry{next, v.index})
	}
	return v.value, nil
}
func (m *merger) close() {
	for _, r := range m.readers {
		r.file.Close()
	}
}

// Ranks uses zero-based ranks. It consumes every run to verify complete hashes,
// even after the requested ranks have been found. Fan-in bounds open descriptors
// and memory independently of the total input count.
func (s *Sorter) Ranks(ctx context.Context, ranks []uint64) (map[uint64]*big.Rat, error) {
	if s.closed {
		return nil, fmt.Errorf("sorter_closed")
	}
	for _, r := range ranks {
		if r >= s.rows {
			return nil, fmt.Errorf("rank_out_of_range")
		}
	}
	if e := s.flush(ctx); e != nil {
		return nil, e
	}
	for len(s.files) > s.options.FanIn {
		var merged []file
		for begin := 0; begin < len(s.files); begin += s.options.FanIn {
			group := s.files[begin:min(begin+s.options.FanIn, len(s.files))]
			if len(group) == 1 {
				merged = append(merged, group[0])
				continue
			}
			m, e := newMerger(group)
			if e != nil {
				return nil, e
			}
			out, e := s.write(ctx, m.next)
			m.close()
			if e != nil {
				return nil, e
			}
			merged = append(merged, out)
			for _, old := range group {
				if e = os.Remove(old.path); e != nil {
					return nil, e
				}
				s.options.Budget.Used -= old.segment.Bytes
			}
		}
		s.files = merged
	}
	m, e := newMerger(s.files)
	if e != nil {
		return nil, e
	}
	defer m.close()
	values := map[uint64]*big.Rat{}
	for _, r := range ranks {
		values[r] = nil
	}
	var count uint64
	for {
		if count%256 == 0 {
			if e = ctx.Err(); e != nil {
				return nil, e
			}
		}
		v, e := m.next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, e
		}
		if _, ok := values[count]; ok {
			values[count] = new(big.Rat).Set(v)
		}
		count++
	}
	if count != s.rows {
		return nil, fmt.Errorf("sort_count_mismatch")
	}
	return values, nil
}
func (s *Sorter) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	defer s.lock.Close()
	var bytes uint64
	entries, e := os.ReadDir(s.directory)
	if e != nil {
		return e
	}
	for _, entry := range entries {
		info, e := entry.Info()
		if e != nil {
			return e
		}
		bytes += uint64(info.Size())
	}
	s.lock.Close()
	e = os.RemoveAll(s.directory)
	if e == nil {
		s.options.Budget.Used -= min(bytes, s.options.Budget.Used)
	}
	return e
}
