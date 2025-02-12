package aws

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewArn(t *testing.T) {
	a, err := NewARN("arn:aws:ec2:eu-west-3:123456789000:instance/i-abcdefghijklmnopq")
	assert.NoError(t, err)
	assert.Equal(t, a.Partition, "aws")
	assert.Equal(t, a.Service, "ec2")
	assert.Equal(t, a.Region, "eu-west-3")
	assert.Equal(t, a.AccountID, "123456789000")
	assert.Equal(t, a.ResourceType, "instance")
	assert.Equal(t, a.ResourceID, "i-abcdefghijklmnopq")

	a, err = NewARN("arn:aws:s3:::abcdefghijklmnop")
	assert.NoError(t, err)
	assert.Equal(t, a.Partition, "aws")
	assert.Equal(t, a.Service, "s3")
	assert.Equal(t, a.Region, "")
	assert.Equal(t, a.AccountID, "")
	assert.Equal(t, a.ResourceType, "")
	assert.Equal(t, a.ResourceID, "abcdefghijklmnop")
}
