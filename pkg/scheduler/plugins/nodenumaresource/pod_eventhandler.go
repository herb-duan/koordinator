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

package nodenumaresource

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext"
	frameworkexthelper "github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext/helper"
	"github.com/koordinator-sh/koordinator/pkg/util"
	"github.com/koordinator-sh/koordinator/pkg/util/cpuset"
	reservationutil "github.com/koordinator-sh/koordinator/pkg/util/reservation"
)

type podEventHandler struct {
	resourceManager ResourceManager
}

func registerPodEventHandler(handle framework.Handle, resourceManager ResourceManager) {
	podInformer := handle.SharedInformerFactory().Core().V1().Pods().Informer()
	eventHandler := &podEventHandler{
		resourceManager: resourceManager,
	}
	frameworkexthelper.ForceSyncFromInformer(context.TODO().Done(), handle.SharedInformerFactory(), podInformer, eventHandler)
	extendedHandle, ok := handle.(frameworkext.ExtendedHandle)
	if ok {
		extendedHandle.RegisterForgetPodHandler(eventHandler.deletePod)
		reservationInformer := extendedHandle.KoordinatorSharedInformerFactory().Scheduling().V1alpha1().Reservations()
		reservationEventHandler := reservationutil.NewReservationToPodEventHandler(eventHandler, reservationutil.IsObjValidActiveReservation)
		frameworkexthelper.ForceSyncFromInformer(context.TODO().Done(), extendedHandle.KoordinatorSharedInformerFactory(), reservationInformer.Informer(), reservationEventHandler)
	}
}

func (c *podEventHandler) OnAdd(obj interface{}, isInInitialList bool) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return
	}
	c.updatePod(nil, pod)
}

func (c *podEventHandler) OnUpdate(oldObj, newObj interface{}) {
	oldPod, ok := oldObj.(*corev1.Pod)
	if !ok {
		return
	}

	pod, ok := newObj.(*corev1.Pod)
	if !ok {
		return
	}
	c.updatePod(oldPod, pod)
}

func (c *podEventHandler) OnDelete(obj interface{}) {
	var pod *corev1.Pod
	switch t := obj.(type) {
	case *corev1.Pod:
		pod = t
	case cache.DeletedFinalStateUnknown:
		var ok bool
		pod, ok = t.Obj.(*corev1.Pod)
		if !ok {
			return
		}
	default:
		break
	}

	if pod == nil {
		return
	}
	c.deletePod(pod)
}

func (c *podEventHandler) updatePod(oldPod, pod *corev1.Pod) {
	if pod.Spec.NodeName == "" {
		return
	}
	if util.IsPodTerminated(pod) {
		c.deletePod(pod)
		return
	}

	resourceStatus, err := extension.GetResourceStatus(pod.Annotations)
	if err != nil {
		return
	}
	resourceSpec, err := extension.GetResourceSpec(pod.Annotations)
	if err != nil {
		return
	}

	cpus, err := cpuset.Parse(resourceStatus.CPUSet)
	if err != nil {
		return
	}
	if len(resourceStatus.NUMANodeResources) == 0 && cpus.IsEmpty() {
		return
	}

	allocation := &PodAllocation{
		UID:                pod.UID,
		Namespace:          pod.Namespace,
		Name:               pod.Name,
		CPUSet:             cpus,
		CPUExclusivePolicy: resourceSpec.PreferredCPUExclusivePolicy,
		NUMANodeResources:  make([]NUMANodeResource, 0, len(resourceStatus.NUMANodeResources)),
	}
	for _, numaNodeRes := range resourceStatus.NUMANodeResources {
		allocation.NUMANodeResources = append(allocation.NUMANodeResources, NUMANodeResource{
			Node:      int(numaNodeRes.Node),
			Resources: numaNodeRes.Resources,
		})
	}

	c.resourceManager.Update(pod.Spec.NodeName, allocation)
}

func (c *podEventHandler) deletePod(pod *corev1.Pod) {
	if pod.Spec.NodeName == "" {
		return
	}

	c.resourceManager.Release(pod.Spec.NodeName, pod.UID)
}
