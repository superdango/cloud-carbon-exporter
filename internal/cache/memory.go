package cache

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/superdango/cloud-carbon-exporter/internal/must"
)

type DynamicValueFunc func(ctx context.Context) (any, error)

type entry struct {
	expiresAt     time.Time
	v             any
	dynamicFunc   DynamicValueFunc
	cacheDuration time.Duration
}

func (e *entry) isExpired() bool {
	return time.Since(e.expiresAt) > 0
}

func (e *entry) isDynamic() bool {
	return e.dynamicFunc != nil
}

func (e *entry) refresh(ctx context.Context) error {
	v, err := e.dynamicFunc(ctx)
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

	if fn, ok := v.(DynamicValueFunc); ok {
		// store dynamic value as expired to force refresh on the first Get
		m.m.Store(k, &entry{
			expiresAt:     time.Now(),
			v:             v,
			cacheDuration: defaultTTL,
			dynamicFunc:   fn,
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

// SetDynamic creates cache entry with dynamic value.
func (m *Memory) SetDynamic(ctx context.Context, k string, fn DynamicValueFunc, ttl ...time.Duration) error {
	defaultTTL := m.defaultTTL
	if len(ttl) > 0 {
		defaultTTL = ttl[0]
	}

	m.m.Store(k, &entry{
		expiresAt:     time.Now(),
		cacheDuration: defaultTTL,
		dynamicFunc:   fn,
	})

	slog.Debug("new dynamic cache entry", "key", k)

	return nil
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

func (m *Memory) Exists(ctx context.Context, k string) (bool, error) {
	_, found := m.m.Load(k)
	return found, nil
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
