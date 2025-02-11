package aws

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewArn(t *testing.T) {
	a, err := newArn("arn:aws:ec2:eu-west-3:123456789000:instance/i-abcdefghijklmnopq")
	assert.NoError(t, err)
	assert.Equal(t, a.namespace, "aws")
	assert.Equal(t, a.service, "ec2")
	assert.Equal(t, a.region, "eu-west-3")
	assert.Equal(t, a.accountID, "123456789000")
	assert.Equal(t, a.kind, "instance")
	assert.Equal(t, a.id, "i-abcdefghijklmnopq")

	a, err = newArn("arn:aws:s3:::abcdefghijklmnop")
	assert.NoError(t, err)
	assert.Equal(t, a.namespace, "aws")
	assert.Equal(t, a.service, "s3")
	assert.Equal(t, a.region, "")
	assert.Equal(t, a.accountID, "")
	assert.Equal(t, a.kind, "")
	assert.Equal(t, a.id, "abcdefghijklmnop")
}
