package cloudcarbonexporter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetMetricLabel(t *testing.T) {
	m := new(Metric)

	m.AddLabel("foo", "bar")
	assert.Equal(t, "bar", m.Labels["foo"])

	m.AddLabel("foo", "baz")
	assert.Equal(t, "baz", m.Labels["foo"])

	m.AddLabel("zoo", "zaz")
	assert.Equal(t, "baz", m.Labels["foo"])
	assert.Equal(t, "zaz", m.Labels["zoo"])

	assert.Len(t, m.Labels, 2)
}
