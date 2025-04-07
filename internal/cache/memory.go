package cache

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/superdango/cloud-carbon-exporter/internal/must"
)

type DynamicValue func(ctx context.Context) (any, error)

type entry struct {
	expiresAt     time.Time
	v             any
	fn            DynamicValue
	cacheDuration time.Duration
}

func (e *entry) isExpired() bool {
	return time.Since(e.expiresAt) > 0
}

func (e *entry) isDynamic() bool {
	return e.fn != nil
}

func (e *entry) refresh(ctx context.Context) error {
	v, err := e.fn(ctx)
	if err != nil {
		return err
	}
	e.v = v
	e.expiresAt = time.Now().Add(e.cacheDuration)
	return nil
}

type Memory struct {
	m          *sync.Map
	defaultTTL time.Duration
}

func NewMemory(ctx context.Context, defaultTTL time.Duration) *Memory {
	cache := &Memory{
		m:          new(sync.Map),
		defaultTTL: defaultTTL,
	}

	go cache.expirerer(ctx)

	return cache
}

func (m *Memory) Set(ctx context.Context, k string, v any, ttl ...time.Duration) error {
	defaultTTL := m.defaultTTL
	if len(ttl) > 0 {
		defaultTTL = ttl[0]
	}

	if fn, ok := v.(DynamicValue); ok {
		// store dynamic value as expired to force refresh on the first Get
		m.m.Store(k, &entry{
			expiresAt:     time.Now(),
			v:             v,
			fn:            fn,
			cacheDuration: defaultTTL,
		})

		slog.Debug("new dynamic cache entry", "key", k)

		return nil
	}

	m.m.Store(k, &entry{
		expiresAt:     time.Now().Add(defaultTTL),
		v:             v,
		cacheDuration: defaultTTL,
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

	entry, ok := v.(*entry)
	must.Assert(ok, "loaded value is not an entry")

	if entry.isExpired() && !entry.isDynamic() {
		slog.Debug("cache expired", "key", k)
		m.m.Delete(k)
		return nil, ErrNotFound
	}

	if entry.isExpired() && entry.isDynamic() {
		if err := entry.refresh(ctx); err != nil {
			return nil, err
		}
		slog.Debug("dynamic entry refreshed", "key", k)
	}

	return entry.v, nil
}

func (m *Memory) expirerer(ctx context.Context) {
	for {
		<-time.Tick(time.Second)

		m.m.Range(func(k, v any) bool {
			entry, ok := v.(*entry)
			must.Assert(ok, "loaded value is not an entry")

			if entry.isExpired() && !entry.isDynamic() {
				slog.Debug("cache expired", "key", k)
				m.m.Delete(k)
			}

			if entry.isExpired() && entry.isDynamic() {
				if err := entry.refresh(ctx); err != nil {
					slog.Warn("failed to refresh dynamic entry", "key", k, "err", err.Error())
				}
				entry.expiresAt = time.Now().Add(entry.cacheDuration)
			}

			return true
		})
	}

}
