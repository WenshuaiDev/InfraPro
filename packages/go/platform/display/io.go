package display

import (
	"fmt"
	"io"
)

func readBounded(r io.Reader, size int64) ([]byte, error) {
	if size < 0 || size > 32<<20 {
		return nil, fmt.Errorf("file budget exceeded")
	}
	b, e := io.ReadAll(io.LimitReader(r, size+1))
	if e == nil && int64(len(b)) != size {
		e = fmt.Errorf("file size mismatch")
	}
	return b, e
}
