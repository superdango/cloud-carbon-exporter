package cloud

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestObjectStoragePower(t *testing.T) {
	assert.Equal(t, 0.01197, EstimateObjectStorage(10))
}
