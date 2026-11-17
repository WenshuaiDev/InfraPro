package access

import "github.com/jackc/pgx/v5"

type memoTransaction struct {
	pgx.Tx
	values map[any]any
}

// MemoTx avoids repeated read-only projections inside one transaction. Callers
// must hold the owning mutation lock and must not mutate the cached data in that
// transaction. It never shares authorization across transactions or requests.
// pgx transactions, and this memo, are used by one goroutine at a time.
func MemoTx[T any](tx pgx.Tx, key any, read func() (T, error)) (T, error) {
	cache, ok := tx.(*memoTransaction)
	if ok {
		if value, exists := cache.values[key]; exists {
			return value.(T), nil
		}
	}
	value, err := read()
	if ok && err == nil {
		cache.values[key] = value
	}
	return value, err
}
