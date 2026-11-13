package exactsort

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Checkpoint flushes all accepted values before returning immutable segment
// identities. The caller commits these together with its aggregate/source cursor.
func (s *Sorter) Checkpoint(ctx context.Context) ([]Segment, error) {
	if e := s.flush(ctx); e != nil {
		return nil, e
	}
	out := make([]Segment, 0, len(s.files))
	for _, f := range s.files {
		out = append(out, f.segment)
	}
	return out, nil
}

// Restore imports exact sealed runs without sorting them again. The stream is
// bounded by the recorded size and verified through EOF for ordering/hash/count.
func (s *Sorter) Restore(ctx context.Context, segments []Segment, fetch func(context.Context, Segment) (io.ReadCloser, error)) error {
	if s.rows != 0 || len(s.files) != 0 || s.closed {
		return fmt.Errorf("sort_restore_requires_empty_state")
	}
	for _, segment := range segments {
		if segment.Rows == 0 || segment.Bytes == 0 || segment.Bytes > s.options.Budget.Limit {
			return fmt.Errorf("invalid_sort_checkpoint")
		}
		if e := s.restoreFile(ctx, segment, fetch); e != nil {
			return e
		}
	}
	return nil
}

func (s *Sorter) restoreFile(ctx context.Context, segment Segment, fetch func(context.Context, Segment) (io.ReadCloser, error)) (err error) {
	defer func() { err = temporaryError(err) }()
	in, e := fetch(ctx, segment)
	if e != nil {
		return e
	}
	defer in.Close()
	s.sequence++
	path := filepath.Join(s.directory, fmt.Sprintf("%020d.rat", s.sequence))
	out, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	writer := &quotaWriter{writer: out, budget: s.options.Budget}
	defer func() {
		out.Close()
		if err != nil {
			os.Remove(path)
			s.options.Budget.Used -= writer.written
		}
	}()
	buffer := make([]byte, 64<<10)
	limited := io.LimitReader(in, int64(segment.Bytes)+1)
	for {
		if e = ctx.Err(); e != nil {
			return e
		}
		n, readError := limited.Read(buffer)
		if n > 0 {
			if _, e = writer.Write(buffer[:n]); e != nil {
				return e
			}
		}
		if readError == io.EOF {
			break
		}
		if readError != nil {
			return readError
		}
	}
	if writer.written != segment.Bytes {
		return fmt.Errorf("sort_segment_integrity_unavailable")
	}
	if e = out.Close(); e != nil {
		return e
	}
	f := file{path, segment}
	r, e := open(f)
	if e != nil {
		return e
	}
	defer r.file.Close()
	for i := uint64(0); ; i++ {
		if i%256 == 0 {
			if e = ctx.Err(); e != nil {
				return e
			}
		}
		_, e = r.next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return e
		}
	}
	if ^uint64(0)-s.rows < segment.Rows {
		return fmt.Errorf("sort_count_overflow")
	}
	s.files = append(s.files, f)
	s.rows += segment.Rows
	return nil
}
