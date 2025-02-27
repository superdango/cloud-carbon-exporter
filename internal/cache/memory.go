package cache

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/superdango/cloud-carbon-exporter/internal/must"
)

type entry struct {
	expiresAt time.Time
	v         any
}

type Memory struct {
	m          *sync.Map
	defaultTTL time.Duration
}

func NewMemory(defaultTTL time.Duration) *Memory {
	cache := &Memory{
		m:          new(sync.Map),
		defaultTTL: defaultTTL,
	}

	go cache.expirerer()

	return cache
}

func (m *Memory) Set(ctx context.Context, k string, v any, ttl ...time.Duration) error {
	defaultTTL := m.defaultTTL
	if len(ttl) > 0 {
		defaultTTL = ttl[0]
	}

	m.m.Store(k, entry{
		expiresAt: time.Now().Add(defaultTTL),
		v:         v,
	})

	slog.Debug("new cache entry", "key", k)
	return nil
}

func (m *Memory) GetOrSet(ctx context.Context, key string, valueFunc func(ctx context.Context) (any, error), ttl ...time.Duration) (v any, err error) {
	v, err = m.Get(ctx, key)
	if err != nil {
		if err == ErrNotFound {
			v, err = valueFunc(ctx)
			if err != nil {
				return nil, err
			}

			err = m.Set(ctx, key, v, ttl...)
			if err != nil {
				return nil, err
			}

			return v, nil
		}
		return nil, err
	}

	return v, nil
}

func (m *Memory) Get(ctx context.Context, k string) (v any, err error) {
	v, found := m.m.Load(k)
	if !found {
		return nil, ErrNotFound
	}

	entry, ok := v.(entry)
	must.Assert(ok, "loaded value is not an entry")

	if time.Since(entry.expiresAt) > 0 {
		slog.Debug("cache expired", "key", k)
		m.m.Delete(k)
		return nil, ErrNotFound
	}

	return entry.v, nil
}

func (m *Memory) expirerer() {
	for {
		<-time.Tick(time.Second)

		m.m.Range(func(k, v any) bool {
			entry, ok := v.(entry)
			must.Assert(ok, "loaded value is not an entry")

			if time.Since(entry.expiresAt) > 0 {
				slog.Debug("cache expired", "key", k)
				m.m.Delete(k)
			}

			return true
		})
	}

}
