/*
Copyright 2022 The Koordinator Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/scheduler/apis/config"
	schedconfig "k8s.io/kubernetes/pkg/scheduler/apis/config"

	"github.com/koordinator-sh/koordinator/apis/extension"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// LoadAwareSchedulingArgs holds arguments used to configure the LoadAwareScheduling plugin.
type LoadAwareSchedulingArgs struct {
	metav1.TypeMeta

	// FilterExpiredNodeMetrics indicates whether to filter nodes where koordlet fails to update NodeMetric.
	// Deprecated: NodeMetric should always be checked for expiration.
	FilterExpiredNodeMetrics *bool
	// NodeMetricExpirationSeconds indicates the NodeMetric expiration in seconds.
	// When NodeMetrics expired, the node is considered abnormal.
	// Default is 180 seconds.
	NodeMetricExpirationSeconds *int64
	// EnableScheduleWhenNodeMetricsExpired Indicates whether nodes with expired nodeMetrics are allowed to schedule pods.
	EnableScheduleWhenNodeMetricsExpired *bool
	// ResourceWeights indicates the weights of resources.
	// The weights of CPU and Memory are both 1 by default.
	ResourceWeights map[corev1.ResourceName]int64
	// UsageThresholds indicates the resource utilization threshold of the whole machine.
	// The default for CPU is 65%, and the default for memory is 95%.
	UsageThresholds map[corev1.ResourceName]int64
	// ProdUsageThresholds indicates the resource utilization threshold of Prod Pods compared to the whole machine.
	// Not enabled by default
	ProdUsageThresholds map[corev1.ResourceName]int64
	// ScoreAccordingProdUsage controls whether to score according to the utilization of Prod Pod
	ScoreAccordingProdUsage bool
	// Estimator indicates the expected Estimator to use
	Estimator string
	// EstimatedScalingFactors indicates the factor when estimating resource usage.
	// The default value of CPU is 85%, and the default value of Memory is 70%.
	EstimatedScalingFactors map[corev1.ResourceName]int64
	// EstimatedSecondsAfterPodScheduled indicates the force estimation duration
	// after pod condition PodScheduled transition to True in seconds.
	EstimatedSecondsAfterPodScheduled *int64
	// EstimatedSecondsAfterInitialized indicates the force estimation duration
	// after pod condition Initialized transition to True in seconds.
	EstimatedSecondsAfterInitialized *int64
	// AllowCustomizeEstimation indicates whether to allow reading estimation args from pod's metadata.
	AllowCustomizeEstimation bool
	// Aggregated supports resource utilization filtering and scoring based on percentile statistics
	Aggregated *LoadAwareSchedulingAggregatedArgs
}

type LoadAwareSchedulingAggregatedArgs struct {
	// UsageThresholds indicates the resource utilization threshold of the machine based on percentile statistics
	UsageThresholds map[corev1.ResourceName]int64
	// UsageAggregationType indicates the percentile type of the machine's utilization when filtering
	// If enabled, only one of the slov1alpha1.AggregationType definitions can be used.
	UsageAggregationType extension.AggregationType
	// UsageAggregatedDuration indicates the statistical period of the percentile of the machine's utilization when filtering
	// If no specific period is set, the maximum period recorded by NodeMetrics will be used by default.
	UsageAggregatedDuration metav1.Duration

	// ScoreAggregationType indicates the percentile type of the machine's utilization when scoring
	// If enabled, only one of the slov1alpha1.AggregationType definitions can be used.
	ScoreAggregationType extension.AggregationType
	// ScoreAggregatedDuration indicates the statistical period of the percentile of Prod Pod's utilization when scoring
	// If no specific period is set, the maximum period recorded by NodeMetrics will be used by default.
	ScoreAggregatedDuration metav1.Duration
}

// ScoringStrategyType is a "string" type.
type ScoringStrategyType string

const (
	// MostAllocated strategy favors node with the least amount of available resource
	MostAllocated ScoringStrategyType = "MostAllocated"
	// BalancedAllocation strategy favors nodes with balanced resource usage rate
	BalancedAllocation ScoringStrategyType = "BalancedAllocation"
	// LeastAllocated strategy favors node with the most amount of available resource
	LeastAllocated ScoringStrategyType = "LeastAllocated"
)

// ScoringStrategy define ScoringStrategyType for the plugin
type ScoringStrategy struct {
	// Type selects which strategy to run.
	Type ScoringStrategyType

	// Resources a list of pairs <resource, weight> to be considered while scoring
	// allowed weights start from 1.
	Resources []schedconfig.ResourceSpec
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NodeNUMAResourceArgs holds arguments used to configure the NodeNUMAResource plugin.
type NodeNUMAResourceArgs struct {
	metav1.TypeMeta

	// DefaultCPUBindPolicy represents the default CPU bind policy.
	// If it is empty and the Pod does not declare a binding policy,
	// the core will not be bound to the LSE/LSR type Pod.
	DefaultCPUBindPolicy CPUBindPolicy
	// ScoringStrategy is used to configure the scoring strategy of the Node-level.
	ScoringStrategy *ScoringStrategy
	// NUMAScoringStrategy is used to configure the scoring strategy of the NUMANode-level
	NUMAScoringStrategy *ScoringStrategy
}

// CPUBindPolicy defines the CPU binding policy
type CPUBindPolicy = string

const (
	// CPUBindPolicyDefault performs the default bind policy that specified in koord-scheduler configuration
	CPUBindPolicyDefault = CPUBindPolicy(extension.CPUBindPolicyDefault)
	// CPUBindPolicyFullPCPUs favor cpuset allocation that pack in few physical cores
	CPUBindPolicyFullPCPUs = CPUBindPolicy(extension.CPUBindPolicyFullPCPUs)
	// CPUBindPolicySpreadByPCPUs favor cpuset allocation that evenly allocate logical cpus across physical cores
	CPUBindPolicySpreadByPCPUs = CPUBindPolicy(extension.CPUBindPolicySpreadByPCPUs)
	// CPUBindPolicyConstrainedBurst constrains the CPU Shared Pool range of the Burstable Pod
	CPUBindPolicyConstrainedBurst = CPUBindPolicy(extension.CPUBindPolicyConstrainedBurst)
)

type CPUExclusivePolicy = extension.CPUExclusivePolicy

const (
	// CPUExclusivePolicyNone does not perform any exclusive policy
	CPUExclusivePolicyNone CPUExclusivePolicy = extension.CPUExclusivePolicyNone
	// CPUExclusivePolicyPCPULevel represents mutual exclusion in the physical core dimension
	CPUExclusivePolicyPCPULevel CPUExclusivePolicy = extension.CPUExclusivePolicyPCPULevel
	// CPUExclusivePolicyNUMANodeLevel indicates mutual exclusion in the NUMA topology dimension
	CPUExclusivePolicyNUMANodeLevel CPUExclusivePolicy = extension.CPUExclusivePolicyNUMANodeLevel
)

// NUMAAllocateStrategy indicates how to choose satisfied NUMA Nodes during binding CPUs
type NUMAAllocateStrategy = extension.NUMAAllocateStrategy

const (
	// NUMAMostAllocated indicates that allocates from the NUMA Node with the least amount of available resource.
	NUMAMostAllocated NUMAAllocateStrategy = extension.NUMAMostAllocated
	// NUMALeastAllocated indicates that allocates from the NUMA Node with the most amount of available resource.
	NUMALeastAllocated NUMAAllocateStrategy = extension.NUMALeastAllocated
	// NUMADistributeEvenly indicates that evenly distribute CPUs across NUMA Nodes.
	NUMADistributeEvenly NUMAAllocateStrategy = extension.NUMADistributeEvenly
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ReservationArgs holds arguments used to configure the Reservation plugin.
type ReservationArgs struct {
	metav1.TypeMeta

	// EnablePreemption indicates whether to enable preemption for reservations.
	EnablePreemption bool
	// MinCandidateNodesPercentage is the minimum number of candidates to
	// shortlist when dry running preemption as a percentage of number of nodes.
	// Must be in the range [0, 100]. Defaults to 10% of the cluster size if
	// unspecified.
	MinCandidateNodesPercentage int32
	// MinCandidateNodesAbsolute is the absolute minimum number of candidates to
	// shortlist. The likely number of candidates enumerated for dry running
	// preemption is given by the formula:
	// numCandidates = max(numNodes * minCandidateNodesPercentage, minCandidateNodesAbsolute)
	// We say "likely" because there are other factors such as PDB violations
	// that play a role in the number of candidates shortlisted. Must be at least
	// 0 nodes. Defaults to 100 nodes if unspecified.
	MinCandidateNodesAbsolute int32
	// Workers number of reservation controller.
	// Defaults to 1 if unspecified.
	ControllerWorkers int32
	// GCDurationSeconds is the duration in seconds after which expired or succeeded reservations
	// will be garbage collected. Defaults to 24 hours (86400 seconds) if unspecified.
	// This value should be provided in seconds.
	GCDurationSeconds int64
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ElasticQuotaArgs holds arguments used to configure the ElasticQuota plugin.
type ElasticQuotaArgs struct {
	metav1.TypeMeta

	// DelayEvictTime is the duration to handle the jitter of used and runtime
	DelayEvictTime metav1.Duration

	// RevokePodInterval is the interval to check quotaGroup's used and runtime
	RevokePodInterval metav1.Duration

	// DefaultQuotaGroupMax limit the maxQuota of DefaultQuotaGroup
	DefaultQuotaGroupMax corev1.ResourceList

	// SystemQuotaGroupMax limit the maxQuota of SystemQuotaGroup
	SystemQuotaGroupMax corev1.ResourceList

	// QuotaGroupNamespace is the namespace of the DefaultQuotaGroup/SystemQuotaGroup
	QuotaGroupNamespace string

	// MonitorAllQuotas monitor the quotaGroups' used and runtime Quota to revoke pods
	MonitorAllQuotas bool

	// EnableCheckParentQuota check parentQuotaGroups' used and runtime Quota in PreFilter
	EnableCheckParentQuota bool

	// EnableRuntimeQuota if false, use max instead of runtime for all checks.
	EnableRuntimeQuota bool

	// EnableMinQuotaScale if true, min will be scaled when minQuotaSum > totalRes.
	EnableMinQuotaScale bool

	// DisableDefaultQuotaPreemption if true, will not preempt pods in default quota.
	DisableDefaultQuotaPreemption bool

	// HookPlugins is expected to be configured with enabled hook plugins
	HookPlugins []HookPluginConf
}

// HookPluginConf define configuration for a single hook plugin
type HookPluginConf struct {
	// Key is the key of the hook plugin
	Key string

	// FactoryKey is the key of the hook plugin factory
	FactoryKey string

	// FactoryArgs is the arguments of the hook plugin factory
	FactoryArgs string
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CoschedulingArgs defines the parameters for Gang Scheduling plugin.
type CoschedulingArgs struct {
	metav1.TypeMeta

	// DefaultTimeout is the default gang's waiting time in Permit stage
	// default is 600 seconds
	DefaultTimeout metav1.Duration
	// Workers number of controller
	// default is 1
	ControllerWorkers int64
	// Skip check schedule cycle [Deprecated]
	// default is false
	SkipCheckScheduleCycle bool
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DeviceShareArgs defines the parameters for DeviceShare plugin.
type DeviceShareArgs struct {
	metav1.TypeMeta

	// Allocator indicates the expected allocator to use
	// Deprecated: Adapting to different allocators is no longer supported.
	Allocator string
	// ScoringStrategy selects the device resource scoring strategy.
	ScoringStrategy *ScoringStrategy
	// DisableDeviceNUMATopologyAlignment indicates device don't need to align with other resources' numa topology
	DisableDeviceNUMATopologyAlignment bool
	// GPUSharedResourceTemplatesConfig holds configurations for GPU shared resource templates.
	GPUSharedResourceTemplatesConfig *GPUSharedResourceTemplatesConfig
}

type GPUSharedResourceTemplatesConfig struct {
	// ConfigMapNamespace is the namespace of the ConfigMap holding GPU shared resource templates data.
	ConfigMapNamespace string
	// ConfigMapName is the name of the ConfigMap holding GPU shared resource templates data.
	ConfigMapName string
	// MatchedResources are resources matched for GPU shared resource templates.
	// If a GPU shared pod requests any of the matched resources,
	// all of its requested GPU resources would be matched for GPU shared resource templates.
	MatchedResources []corev1.ResourceName
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ScarceResourceAvoidanceArgs defines the parameters for ScarceResourceAvoidance plugin.
type ScarceResourceAvoidanceArgs struct {
	metav1.TypeMeta
	Resources []corev1.ResourceName
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NodeResourcesFitPlusArgs defines the parameters for NodeResourcesFitPlus plugin.
type NodeResourcesFitPlusArgs struct {
	metav1.TypeMeta
	Resources map[corev1.ResourceName]ResourcesType
}

type ResourcesType struct {
	Type   config.ScoringStrategyType
	Weight int64
}
