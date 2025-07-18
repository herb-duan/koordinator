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

package estimator

import (
	"math"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
	resourceapi "k8s.io/kubernetes/pkg/api/v1/resource"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/apis/config"
)

const (
	defaultEstimatorName = "defaultEstimator"

	// DefaultMilliCPURequest defines default milli cpu request number.
	DefaultMilliCPURequest int64 = 250 // 0.25 core
	// DefaultMemoryRequest defines default memory request size.
	DefaultMemoryRequest int64 = 200 * 1024 * 1024 // 200 MB
)

type DefaultEstimator struct {
	resourceWeights map[corev1.ResourceName]int64
	scalingFactors  map[corev1.ResourceName]int64
	allowCustomize  bool
}

func NewDefaultEstimator(args *config.LoadAwareSchedulingArgs, handle framework.Handle) (Estimator, error) {
	return &DefaultEstimator{
		resourceWeights: args.ResourceWeights,
		scalingFactors:  args.EstimatedScalingFactors,
		allowCustomize:  args.AllowCustomizeEstimation,
	}, nil
}

func (e *DefaultEstimator) Name() string {
	return defaultEstimatorName
}

func (e *DefaultEstimator) EstimatePod(pod *corev1.Pod) (map[corev1.ResourceName]int64, error) {
	var factors map[corev1.ResourceName]int64
	if e.allowCustomize {
		factors = extension.GetCustomEstimatedScalingFactors(pod)
	}
	if len(factors) == 0 {
		factors = e.scalingFactors
	} else {
		for k, v := range e.scalingFactors {
			if _, ok := factors[k]; !ok {
				factors[k] = v
			}
		}
	}
	return estimatedPodUsed(pod, e.resourceWeights, factors), nil
}

func estimatedPodUsed(pod *corev1.Pod, resourceWeights map[corev1.ResourceName]int64, scalingFactors map[corev1.ResourceName]int64) map[corev1.ResourceName]int64 {
	requests, limits := resourceapi.PodRequests(pod, resourceapi.PodResourcesOptions{}), resourceapi.PodLimits(pod, resourceapi.PodResourcesOptions{})
	estimatedUsed := make(map[corev1.ResourceName]int64)
	priorityClass := extension.GetPodPriorityClassWithDefault(pod)
	for resourceName := range resourceWeights {
		realResourceName := extension.TranslateResourceNameByPriorityClass(priorityClass, resourceName)
		estimatedUsed[resourceName] = estimatedUsedByResource(requests, limits, realResourceName, scalingFactors[resourceName])
	}
	return estimatedUsed
}

// TODO(joseph): Do we need to differentiate scalingFactor according to Koordinator Priority type?
func estimatedUsedByResource(requests, limits corev1.ResourceList, resourceName corev1.ResourceName, scalingFactor int64) int64 {
	limitQuantity := limits[resourceName]
	requestQuantity := requests[resourceName]
	var quantity resource.Quantity
	if limitQuantity.Cmp(requestQuantity) > 0 {
		quantity = limitQuantity
	} else {
		quantity = requestQuantity
	}

	if quantity.IsZero() {
		switch resourceName {
		case corev1.ResourceCPU, extension.BatchCPU:
			return DefaultMilliCPURequest
		case corev1.ResourceMemory, extension.BatchMemory:
			return DefaultMemoryRequest
		}
		return 0
	}

	var estimatedUsed int64
	switch resourceName {
	case corev1.ResourceCPU:
		estimatedUsed = int64(math.Round(float64(quantity.MilliValue()) * float64(scalingFactor) / 100))
		if limit := limitQuantity.MilliValue(); limit > 0 && estimatedUsed > limit {
			estimatedUsed = limit
		}
	default:
		estimatedUsed = int64(math.Round(float64(quantity.Value()) * float64(scalingFactor) / 100))
		if limit := limitQuantity.Value(); limit > 0 && estimatedUsed > limit {
			estimatedUsed = limit
		}
	}
	return estimatedUsed
}

func (e *DefaultEstimator) EstimateNode(node *corev1.Node) (corev1.ResourceList, error) {
	rawAllocatable, err := extension.GetNodeRawAllocatable(node.Annotations)
	if err != nil {
		return node.Status.Allocatable, nil
	}
	if len(rawAllocatable) == 0 {
		return node.Status.Allocatable, nil
	}
	if quotav1.Equals(rawAllocatable, node.Status.Allocatable) {
		return node.Status.Allocatable, nil
	}
	allocatableCopy := node.Status.Allocatable.DeepCopy()
	if allocatableCopy == nil {
		allocatableCopy = corev1.ResourceList{}
	}
	for k, v := range rawAllocatable {
		allocatableCopy[k] = v
	}
	return allocatableCopy, nil
}
