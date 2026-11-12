package monitoring

import (
	"bufio"
	"cmp"
	"container/heap"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// Temporary space and run buffers are bounded independently of input size.
// Runs are merged with fixed fan-in, including files exceeding the fd budget.
type spool struct {
	dir   string
	bytes int64
}

func newSpool(parent string) (*spool, error) {
	dir, e := os.MkdirTemp(parent, "infrapro-monitoring-")
	return &spool{dir: dir}, e
}
func (s *spool) close() { _ = os.RemoveAll(s.dir) }
func (s *spool) file() (*spoolFile, error) {
	f, e := os.CreateTemp(s.dir, "run-")
	if e != nil {
		return nil, e
	}
	return &spoolFile{File: f, owner: s}, nil
}
func (s *spool) remove(path string) {
	info, e := os.Stat(path)
	if e == nil {
		s.bytes -= info.Size()
	}
	_ = os.Remove(path)
}

type spoolFile struct {
	*os.File
	owner *spool
}

func (f *spoolFile) Write(p []byte) (int, error) {
	if f.owner.bytes+int64(len(p)) > MaxTemporaryBytes {
		return 0, problem("temporary_space_limit", "该任务暂存达到 8 GiB 上限，请拆分源文件后重新导入。", 422)
	}
	n, e := f.File.Write(p)
	f.owner.bytes += int64(n)
	return n, e
}

type rowIterator struct {
	file    io.ReadCloser
	scanner *bufio.Scanner
	row     rawObservation
	err     error
}

func rowsFrom(r io.ReadCloser) *rowIterator {
	scan := bufio.NewScanner(r)
	scan.Buffer(make([]byte, 64*1024), 8*1024*1024)
	return &rowIterator{file: r, scanner: scan}
}
func openRows(path string) (*rowIterator, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	return rowsFrom(f), nil
}
func (i *rowIterator) next() bool {
	if !i.scanner.Scan() {
		i.err = i.scanner.Err()
		return false
	}
	i.err = json.Unmarshal(i.scanner.Bytes(), &i.row)
	return i.err == nil
}
func (i *rowIterator) close() { i.file.Close() }
func compareRaw(a, b rawObservation) int {
	if n := cmp.Compare(a.Channel, b.Channel); n != 0 {
		return n
	}
	if n := cmp.Compare(a.At, b.At); n != 0 {
		return n
	}
	if n := cmp.Compare(a.Line, b.Line); n != 0 {
		return n
	}
	if n := cmp.Compare(a.Column, b.Column); n != 0 {
		return n
	}
	return cmp.Compare(a.Observation, b.Observation)
}
func monthOf(r rawObservation) int {
	tm := time.Unix(0, r.At).UTC()
	return tm.Year()*100 + int(tm.Month())
}
func compareChunk(a, b rawObservation) int {
	if n := cmp.Compare(a.Shard, b.Shard); n != 0 {
		return n
	}
	if n := cmp.Compare(monthOf(a), monthOf(b)); n != 0 {
		return n
	}
	return compareRaw(a, b)
}

type rowSorter struct {
	s       *spool
	buffer  []rawObservation
	size    int
	runs    []string
	compare func(rawObservation, rawObservation) int
}

func (s *rowSorter) add(r rawObservation) error {
	s.buffer = append(s.buffer, r)
	s.size += len(encodeJSON(r))
	if len(s.buffer) >= 16384 || s.size >= 16*1024*1024 {
		return s.flush()
	}
	return nil
}
func (s *rowSorter) flush() error {
	if len(s.buffer) == 0 {
		return nil
	}
	slices.SortFunc(s.buffer, s.compare)
	f, e := s.s.file()
	if e != nil {
		return e
	}
	writer := bufio.NewWriterSize(f, 64*1024)
	enc := json.NewEncoder(writer)
	for _, r := range s.buffer {
		if e = enc.Encode(r); e != nil {
			f.Close()
			return e
		}
	}
	if e = writer.Flush(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	s.runs = append(s.runs, f.Name())
	s.buffer = nil
	s.size = 0
	return nil
}
func (s *rowSorter) finish(ctx context.Context) (string, error) {
	if e := s.flush(); e != nil {
		return "", e
	}
	if len(s.runs) == 0 {
		f, e := s.s.file()
		if e != nil {
			return "", e
		}
		return f.Name(), f.Close()
	}
	for len(s.runs) > 1 {
		next := []string{}
		for begin := 0; begin < len(s.runs); begin += 32 {
			group := s.runs[begin:min(begin+32, len(s.runs))]
			f, e := s.s.file()
			if e != nil {
				return "", e
			}
			writer := bufio.NewWriterSize(f, 64*1024)
			enc := json.NewEncoder(writer)
			e = mergeRuns(ctx, group, s.compare, func(r rawObservation) error { return enc.Encode(r) })
			if e == nil {
				e = writer.Flush()
			}
			f.Close()
			if e != nil {
				return "", e
			}
			for _, path := range group {
				s.s.remove(path)
			}
			next = append(next, f.Name())
		}
		s.runs = next
	}
	return s.runs[0], nil
}

type mergeItem struct {
	row   rawObservation
	index int
}
type mergeHeap struct {
	items   []mergeItem
	compare func(rawObservation, rawObservation) int
}

func (h mergeHeap) Len() int           { return len(h.items) }
func (h mergeHeap) Less(i, j int) bool { return h.compare(h.items[i].row, h.items[j].row) < 0 }
func (h mergeHeap) Swap(i, j int)      { h.items[i], h.items[j] = h.items[j], h.items[i] }
func (h *mergeHeap) Push(x any)        { h.items = append(h.items, x.(mergeItem)) }
func (h *mergeHeap) Pop() any {
	n := len(h.items) - 1
	x := h.items[n]
	h.items = h.items[:n]
	return x
}
func mergeRuns(ctx context.Context, paths []string, compare func(rawObservation, rawObservation) int, consume func(rawObservation) error) error {
	its := []*rowIterator{}
	defer func() {
		for _, it := range its {
			it.close()
		}
	}()
	h := &mergeHeap{compare: compare}
	for i, path := range paths {
		it, e := openRows(path)
		if e != nil {
			return e
		}
		its = append(its, it)
		if it.next() {
			heap.Push(h, mergeItem{it.row, i})
		} else if it.err != nil {
			return it.err
		}
	}
	for h.Len() > 0 {
		if e := ctx.Err(); e != nil {
			return e
		}
		v := heap.Pop(h).(mergeItem)
		if e := consume(v.row); e != nil {
			return e
		}
		it := its[v.index]
		if it.next() {
			heap.Push(h, mergeItem{it.row, v.index})
		} else if it.err != nil {
			return it.err
		}
	}
	return nil
}
func (s *spool) object(ctx context.Context, storage *Storage, path, key string) error {
	if filepath.Dir(path) != s.dir {
		return fmt.Errorf("invalid spool path")
	}
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	stat, e := f.Stat()
	if e != nil {
		return e
	}
	return storage.put(ctx, key, f, stat.Size(), "application/x-ndjson")
}
