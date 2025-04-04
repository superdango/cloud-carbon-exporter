package gcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestZonesRegions(t *testing.T) {
	
}

func TestURLFragments(t *testing.T) {
	assert.Equal(t, "bar", fragmentURLPath("http://test.com/foo/bar")[1])
	assert.Equal(t, "bar", fragmentURLPath("http://test.com/foo/bar/baz?raw=true")[1])
	assert.Equal(t, "", fragmentURLPath("http://test.com/")[0])
	assert.Equal(t, "", lastURLPathFragment("http://test.com/"))
	assert.Equal(t, "bob", lastURLPathFragment("http://test.com/bob"))
}
