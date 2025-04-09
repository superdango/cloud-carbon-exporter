package cache

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMemory(t *testing.T) {
	slog.SetLogLoggerLevel(slog.LevelDebug)

	memory := NewMemory(t.Context(), 1*time.Second)
	err := memory.Set(t.Context(), "k1", "v1", 0*time.Second)
	assert.NoError(t, err)

	// should be expired as TTL is 0 second
	_, err = memory.Get(t.Context(), "k1")
	assert.ErrorIs(t, err, ErrNotFound)

	err = memory.Set(t.Context(), "d1", DynamicValueFunc(func(ctx context.Context) (any, error) {
		return "v1", nil
	}))
	assert.NoError(t, err)

	v, err := memory.Get(t.Context(), "d1")
	assert.NoError(t, err)
	assert.Equal(t, "v1", v)

	err = memory.Set(t.Context(), "d2", DynamicValueFunc(func(ctx context.Context) (any, error) {
		return "v1", nil
	}), 0)
	assert.NoError(t, err)

	// even if d2 is expired, value is dynamically refreshed
	v, err = memory.Get(t.Context(), "d2")
	assert.NoError(t, err)
	assert.Equal(t, "v1", v)

	err = memory.Set(t.Context(), "d3", DynamicValueFunc(func(ctx context.Context) (any, error) {
		return nil, fmt.Errorf("expected error")
	}), 0)
	assert.NoError(t, err)

	_, err = memory.Get(t.Context(), "d3")
	assert.Error(t, err)

}
