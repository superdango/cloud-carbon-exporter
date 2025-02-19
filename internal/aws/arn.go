package aws

import (
	"fmt"
	"strings"
)

type ARN struct {
	Partition    string
	Service      string
	Region       string
	AccountID    string
	ResourceType string
	ResourceID   string
}

func NewARN(arnstr string) (*ARN, error) {
	arn := &ARN{}

	splitted := strings.Split(arnstr, ":")
	if len(splitted) != 6 {
		return nil, fmt.Errorf("invalid arn format: %s", arnstr)
	}
	arn.Partition = splitted[1]
	arn.Service = splitted[2]
	arn.Region = splitted[3]
	arn.AccountID = splitted[4]

	resourceComponent := strings.SplitN(splitted[5], "/", 2)
	switch len(resourceComponent) {
	case 1:
		arn.ResourceID = resourceComponent[0]
		return arn, nil
	case 2:
		arn.ResourceType = resourceComponent[0]
		arn.ResourceID = resourceComponent[1]
		return arn, nil
	default:
		return nil, fmt.Errorf("invalid arn resource id format: %s", splitted[5])
	}
}

func (a *ARN) FullType() string {
	if a.ResourceType == "" {
		return a.Service
	}

	return fmt.Sprintf("%s/%s", a.Service, a.ResourceType)
}
