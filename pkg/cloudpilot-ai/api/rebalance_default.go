package api

import (
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsproviderv1 "github.com/cloudpilot-ai/lib/pkg/aws/karpenter-provider-aws/apis/v1"
	awscorev1 "github.com/cloudpilot-ai/lib/pkg/aws/karpenter/apis/v1"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	CloudPilotDemoClusterLabelKey  = "cluster.cloudpilot.ai/demo"
	CloudPilotDemoClusterFinalizer = "cluster.cloudpilot.ai/demo-finalizer"
	CloudPilotManagedNodeLabelKey  = "node.cloudpilot.ai/managed"
	CloudPilotManagedTagKey        = "cloudpilot.ai/managed"

	DefaultMinNonSpotReplicas  = 0
	DefaultNodeClassName       = "cloudpilot"
	DefaultGeneralNodePoolName = "cloudpilot-general"
	DefaultGPUNodePoolName     = "cloudpilot-gpu"

	MaxDemoClusterCount = 10
)

var (
	AWSDefaultBlockSize                  = resource.MustParse("20Gi")
	DefaultAWSDisruptionConsolidateAfter = awscorev1.MustParseNillableDuration("60m")
	AWSExpirationNever                   = awscorev1.MustParseNillableDuration("Never")

	DefaultDisruptionBudgetsNodes = "2"

	DefaultAWSBudgetConfig = []awscorev1.Budget{
		{
			Nodes: DefaultDisruptionBudgetsNodes,
		},
	}
)

func DefaultEC2NodeClassSpec(clusterName string) *awsproviderv1.EC2NodeClassSpec {
	return &awsproviderv1.EC2NodeClassSpec{
		SubnetSelectorTerms: []awsproviderv1.SubnetSelectorTerm{
			{
				Tags: map[string]string{fmt.Sprintf("cluster.cloudpilot.ai/%s", clusterName): "true"},
			},
		},
		SecurityGroupSelectorTerms: []awsproviderv1.SecurityGroupSelectorTerm{
			{
				Tags: map[string]string{fmt.Sprintf("cluster.cloudpilot.ai/%s", clusterName): "true"},
			},
		},
		AMISelectorTerms: []awsproviderv1.AMISelectorTerm{
			{Alias: "al2023@v20250519"},
		},
		BlockDeviceMappings: []*awsproviderv1.BlockDeviceMapping{
			{
				DeviceName: aws.String("/dev/xvda"),
				EBS: &awsproviderv1.BlockDevice{
					Encrypted:  aws.Bool(true),
					VolumeType: aws.String("gp3"),
					VolumeSize: &AWSDefaultBlockSize,
				},
			},
		},
		Kubelet: &awsproviderv1.KubeletConfiguration{
			EvictionHard: map[string]string{
				"memory.available": "10%",
			},
		},
		MetadataOptions: &awsproviderv1.MetadataOptions{
			HTTPPutResponseHopLimit: aws.Int64(2),
		},
		Tags: map[string]string{
			CloudPilotManagedTagKey: "true",
		},
		Role: fmt.Sprintf("CloudPilotNodeRole-%s", clusterName),
	}
}

func DefaultGeneralEC2NodePoolSpec() *awscorev1.NodePoolSpec {
	return &awscorev1.NodePoolSpec{
		Template: awscorev1.NodeClaimTemplate{
			ObjectMeta: awscorev1.ObjectMeta{
				Labels: map[string]string{
					CloudPilotManagedNodeLabelKey: "true",
				},
			},
			Spec: awscorev1.NodeClaimTemplateSpec{
				Requirements: []awscorev1.NodeSelectorRequirementWithMinValues{
					{
						NodeSelectorRequirement: corev1.NodeSelectorRequirement{
							Key:      awsproviderv1.LabelInstanceGPUCount,
							Operator: corev1.NodeSelectorOpDoesNotExist,
						},
					},
					{
						NodeSelectorRequirement: corev1.NodeSelectorRequirement{
							Key:      awsproviderv1.LabelInstanceCategory,
							Operator: corev1.NodeSelectorOpNotIn,
							Values:   []string{"a"},
						},
					},
					{
						NodeSelectorRequirement: corev1.NodeSelectorRequirement{
							Key:      corev1.LabelArchStable,
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{"amd64"},
						},
					},
					{
						NodeSelectorRequirement: corev1.NodeSelectorRequirement{
							Key:      corev1.LabelOSStable,
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{"linux"},
						},
					},
					{
						NodeSelectorRequirement: corev1.NodeSelectorRequirement{
							Key:      awscorev1.CapacityTypeLabelKey,
							Operator: corev1.NodeSelectorOpIn,
							Values:   []string{"spot", "on-demand"},
						},
					},
					{
						NodeSelectorRequirement: corev1.NodeSelectorRequirement{
							Key:      awsproviderv1.LabelInstanceMemory,
							Operator: corev1.NodeSelectorOpLt,
							Values:   []string{"32769"},
						},
					},
					{
						NodeSelectorRequirement: corev1.NodeSelectorRequirement{
							Key:      awsproviderv1.LabelInstanceCPU,
							Operator: corev1.NodeSelectorOpLt,
							Values:   []string{"17"},
						},
					},
					{
						NodeSelectorRequirement: corev1.NodeSelectorRequirement{
							Key:      corev1.LabelInstanceType,
							Operator: corev1.NodeSelectorOpNotIn,
							Values:   []string{"c1.medium", "m1.small"},
						},
					},
				},
				NodeClassRef: &awscorev1.NodeClassReference{
					Group: "karpenter.k8s.aws",
					Kind:  "EC2NodeClass",
					Name:  DefaultNodeClassName,
				},
				ExpireAfter: AWSExpirationNever,
			},
		},
		Disruption: awscorev1.Disruption{
			ConsolidationPolicy: awscorev1.ConsolidationPolicyWhenEmptyOrUnderutilized,
			ConsolidateAfter:    DefaultAWSDisruptionConsolidateAfter,
			Budgets:             DefaultAWSBudgetConfig,
		},
		Weight: aws.Int32(2),
	}
}

// createGPURequirement creates a GPU node selector requirement with the appropriate operator
func createGPURequirement(enableGPU bool) awscorev1.NodeSelectorRequirementWithMinValues {
	return awscorev1.NodeSelectorRequirementWithMinValues{
		NodeSelectorRequirement: corev1.NodeSelectorRequirement{
			Key: awsproviderv1.LabelInstanceGPUCount,
			Operator: lo.Ternary(enableGPU,
				corev1.NodeSelectorOpExists,
				corev1.NodeSelectorOpDoesNotExist),
		},
	}
}

func EnableGPUEC2NodePoolSpec(nodePoolSpec *awscorev1.NodePoolSpec, enableGPU bool) *awscorev1.NodePoolSpec {
	if nodePoolSpec == nil {
		nodePoolSpec = DefaultGeneralEC2NodePoolSpec()
	}

	_, index, found := lo.FindIndexOf(nodePoolSpec.Template.Spec.Requirements, func(req awscorev1.NodeSelectorRequirementWithMinValues) bool {
		return req.Key == awsproviderv1.LabelInstanceGPUCount
	})

	if found {
		// Update existing requirement
		nodePoolSpec.Template.Spec.Requirements[index].Operator = lo.Ternary(enableGPU,
			corev1.NodeSelectorOpExists,
			corev1.NodeSelectorOpDoesNotExist)
		return nodePoolSpec
	}

	// Add new requirement
	nodePoolSpec.Template.Spec.Requirements = append(
		nodePoolSpec.Template.Spec.Requirements,
		createGPURequirement(enableGPU),
	)

	return nodePoolSpec
}

func DefaultGPUEC2NodePoolSpec() *awscorev1.NodePoolSpec {
	return &awscorev1.NodePoolSpec{
		Template: awscorev1.NodeClaimTemplate{
			ObjectMeta: awscorev1.ObjectMeta{
				Labels: map[string]string{
					CloudPilotManagedNodeLabelKey: "true",
				},
			},
			Spec: awscorev1.NodeClaimTemplateSpec{
				Requirements: []awscorev1.NodeSelectorRequirementWithMinValues{
					{
						NodeSelectorRequirement: v1.NodeSelectorRequirement{
							Key:      awsproviderv1.LabelInstanceGPUCount,
							Operator: v1.NodeSelectorOpExists,
						},
					},
					{
						NodeSelectorRequirement: v1.NodeSelectorRequirement{
							Key:      v1.LabelArchStable,
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"amd64"},
						},
					},
					{
						NodeSelectorRequirement: v1.NodeSelectorRequirement{
							Key:      v1.LabelOSStable,
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"linux"},
						},
					},
					{
						NodeSelectorRequirement: v1.NodeSelectorRequirement{
							Key:      awscorev1.CapacityTypeLabelKey,
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"spot", "on-demand"},
						},
					},
				},
				NodeClassRef: &awscorev1.NodeClassReference{
					Group: "karpenter.k8s.aws",
					Kind:  "EC2NodeClass",
					Name:  DefaultNodeClassName,
				},
				ExpireAfter: AWSExpirationNever,
			},
		},
		Disruption: awscorev1.Disruption{
			ConsolidationPolicy: awscorev1.ConsolidationPolicyWhenEmptyOrUnderutilized,
			ConsolidateAfter:    DefaultAWSDisruptionConsolidateAfter,
			Budgets:             DefaultAWSBudgetConfig,
		},
		Weight: aws.Int32(1),
	}
}
