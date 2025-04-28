package cloudcarbonexporter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeLabels(t *testing.T) {
	m := Metric{
		Name: "foo",
		Labels: map[string]string{
			"eks:eks-cluster-name":          "my-cluster",
			"aws:ec2launchtemplate:version": "1.0",
			"karpenter.sh/nodeclaim":        "",
		},
		Value: 1.0,
	}

	assert.Equal(t, map[string]string{
		"eks_eks_cluster_name":          "my-cluster",
		"aws_ec2launchtemplate_version": "1.0",
		"karpenter_sh_nodeclaim":        "",
	}, m.SanitizeLabels().Labels)

}
