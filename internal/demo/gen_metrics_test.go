package demo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNaturalTraficInstant(t *testing.T) {
	assert.Equal(t, 400, naturalTrafficInstant(0, 0, 0))
	assert.Equal(t, 301, naturalTrafficInstant(0, 59, 0))
	assert.Equal(t, 300, naturalTrafficInstant(1, 0, 0))
}
