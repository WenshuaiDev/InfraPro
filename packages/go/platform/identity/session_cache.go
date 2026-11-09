package identity

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

type Cache struct {
	client                          *redis.Client
	prefix                          string
	logger                          *slog.Logger
	hits, misses, failures, retries atomic.Uint64
}

func NewCache(client *redis.Client, prefix string, logger *slog.Logger) *Cache {
	return &Cache{client: client, prefix: prefix, logger: logger}
}
func (c *Cache) Ping(ctx context.Context) error {
	err := c.client.Ping(ctx).Err()
	if err != nil {
		c.failures.Add(1)
	}
	return err
}
func (c *Cache) Get(ctx context.Context, hash string) (*Principal, error) {
	b, e := c.client.Get(ctx, c.prefix+"session:"+hash).Bytes()
	if errors.Is(e, redis.Nil) {
		c.misses.Add(1)
		return nil, nil
	}
	if e != nil {
		c.failures.Add(1)
		return nil, e
	}
	var p Principal
	if e = json.Unmarshal(b, &p); e != nil {
		c.failures.Add(1)
		return nil, e
	}
	c.hits.Add(1)
	p.TokenHash = hash
	return &p, nil
}
func (c *Cache) Put(ctx context.Context, p Principal, ttl time.Duration) error {
	if ttl <= 0 {
		return ErrUnauthorized
	}
	b, e := json.Marshal(p)
	if e != nil {
		return e
	}
	e = c.client.Set(ctx, c.prefix+"session:"+p.TokenHash, b, ttl).Err()
	if e != nil {
		c.failures.Add(1)
	}
	return e
}
func (c *Cache) Delete(ctx context.Context, id, hash string) error {
	e := c.client.Del(ctx, c.prefix+"session:"+hash, c.prefix+"activity:"+id).Err()
	if e != nil {
		c.failures.Add(1)
		c.retries.Add(1)
	}
	return e
}

var rateScript = redis.NewScript(`local n=redis.call('INCR',KEYS[1]); if n==1 then redis.call('PEXPIRE',KEYS[1],ARGV[1]) end; return n`)

func (c *Cache) Rate(ctx context.Context, scope, key string, max int, window time.Duration) error {
	key = c.prefix + "rate:" + scope + ":" + digest(key) + ":" + strconv.FormatInt(time.Now().UnixMilli()/window.Milliseconds(), 10)
	n, e := rateScript.Run(ctx, c.client, []string{key}, window.Milliseconds()).Int()
	if e != nil {
		c.failures.Add(1)
		return e
	}
	if n > max {
		return ErrRate
	}
	return nil
}

type activity struct {
	At      time.Time
	Version int64
	Dirty   bool
}

func (c *Cache) Activity(ctx context.Context, id string) (activity, error) {
	v, e := c.client.HMGet(ctx, c.prefix+"activity:"+id, "at", "version", "dirty").Result()
	if e != nil {
		c.failures.Add(1)
		return activity{}, e
	}
	if v[0] == nil {
		return activity{}, nil
	}
	at, e := strconv.ParseInt(v[0].(string), 10, 64)
	if e != nil {
		return activity{}, e
	}
	version, e := strconv.ParseInt(v[1].(string), 10, 64)
	if e != nil {
		return activity{}, e
	}
	return activity{At: time.UnixMilli(at), Version: version, Dirty: v[2] == "1"}, nil
}

var activityScript = redis.NewScript(`local old=tonumber(redis.call('HGET',KEYS[1],'at') or '0'); if tonumber(ARGV[1])>old then redis.call('HSET',KEYS[1],'at',ARGV[1],'version',ARGV[2],'dirty','1'); redis.call('PEXPIRE',KEYS[1],ARGV[3]); end; return 1`)

func (c *Cache) RecordActivity(ctx context.Context, p Principal, at time.Time, ttl time.Duration) error {
	e := activityScript.Run(ctx, c.client, []string{c.prefix + "activity:" + p.Session.ID}, at.UnixMilli(), p.Version, ttl.Milliseconds()).Err()
	if e != nil {
		c.failures.Add(1)
	}
	return e
}

var activitySavedScript = redis.NewScript(`if redis.call('HGET',KEYS[1],'at')==ARGV[1] then redis.call('HSET',KEYS[1],'dirty','0') end; return 1`)

func (c *Cache) FlushActivities(ctx context.Context, r *Repository, idle time.Duration) error {
	var cursor uint64
	for {
		keys, next, e := c.client.Scan(ctx, cursor, c.prefix+"activity:*", 100).Result()
		if e != nil {
			return e
		}
		for _, key := range keys {
			id := strings.TrimPrefix(key, c.prefix+"activity:")
			a, e := c.Activity(ctx, id)
			if e != nil {
				return e
			}
			if !a.Dirty {
				continue
			}
			if e = r.SaveActivity(ctx, id, a.Version, a.At, idle); e != nil {
				return e
			}
			if e = activitySavedScript.Run(ctx, c.client, []string{key}, a.At.UnixMilli()).Err(); e != nil {
				return e
			}
		}
		cursor = next
		if cursor == 0 {
			return nil
		}
	}
}
func (c *Cache) ClearSessions(ctx context.Context) error {
	for _, kind := range []string{"session:", "activity:"} {
		var cursor uint64
		for {
			keys, next, e := c.client.Scan(ctx, cursor, c.prefix+kind+"*", 100).Result()
			if e != nil {
				return e
			}
			if len(keys) > 0 {
				if e = c.client.Del(ctx, keys...).Err(); e != nil {
					return e
				}
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
	}
	return nil
}
func (c *Cache) LogStats() {
	c.logger.Info("identity cache statistics", "hits", c.hits.Load(), "misses", c.misses.Load(), "failures", c.failures.Load(), "invalidation_retries", c.retries.Load())
}
