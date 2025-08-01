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

package core

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	"github.com/koordinator-sh/koordinator/apis/extension"
	"github.com/koordinator-sh/koordinator/apis/thirdparty/scheduler-plugins/pkg/apis/scheduling/v1alpha1"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/apis/config"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/plugins/coscheduling/util"
)

const (
	ErrPodHasNotBeenAttempted         = "gangGroup %s is scheduling and this pod has not been attempted"
	ErrRepresentativePodAlreadyExists = "representative pod %s of gangGroupID %s already exists"
	ErrPodIsNotExistsInGangCache      = "pod %s is not exists in gangCache"
)

var (
	timeNowFn = time.Now
)

const (
	GangFromPodGroupCrd   string = "GangFromPodGroupCrd"
	GangFromPodAnnotation string = "GangFromPodAnnotation"
)

// Gang  basic podGroup info recorded in gangCache:
type Gang struct {
	Name       string
	WaitTime   time.Duration
	CreateTime time.Time

	// strict-mode or non-strict-mode
	Mode              string
	MinRequiredNumber int
	TotalChildrenNum  int
	GangGroupId       string
	GangGroup         []string

	GangGroupInfo *GangGroupInfo

	Children        map[string]*v1.Pod
	PendingChildren map[string]*v1.Pod
	// pods that have already assumed(waiting in Permit stage)
	WaitingForBindChildren map[string]*v1.Pod
	// pods that have already bound
	BoundChildren map[string]*v1.Pod

	// only-waiting, only consider waiting pods
	// waiting-and-running, consider waiting and running pods
	// once-satisfied, once gang is satisfied, no need to consider any status pods
	GangMatchPolicy string

	GangFrom    string
	HasGangInit bool

	lock sync.RWMutex
}

func NewGang(gangName string) *Gang {
	return &Gang{
		Name:                   gangName,
		CreateTime:             timeNowFn(),
		WaitTime:               0,
		GangGroupId:            gangName,
		GangGroup:              []string{gangName},
		Mode:                   extension.GangModeStrict,
		GangMatchPolicy:        extension.GangMatchPolicyOnceSatisfied,
		Children:               make(map[string]*v1.Pod),
		PendingChildren:        make(map[string]*v1.Pod),
		WaitingForBindChildren: make(map[string]*v1.Pod),
		BoundChildren:          make(map[string]*v1.Pod),
		GangFrom:               GangFromPodAnnotation,
		HasGangInit:            false,
		GangGroupInfo:          NewGangGroupInfo("", nil),
	}
}

func (gang *Gang) tryInitByPodConfig(pod *v1.Pod, args *config.CoschedulingArgs) bool {
	gang.lock.Lock()
	defer gang.lock.Unlock()
	if gang.HasGangInit {
		return false
	}
	minRequiredNumber, err := util.GetGangMinNumFromPod(pod)
	if err != nil {
		klog.Errorf("pod's annotation MinRequiredNumber illegal, gangName: %v, value: %v",
			gang.Name, pod.Annotations[extension.AnnotationGangMinNum])
		return false
	}
	gang.MinRequiredNumber = minRequiredNumber

	totalChildrenNum, err := strconv.ParseInt(pod.Annotations[extension.AnnotationGangTotalNum], 10, 32)
	if err != nil {
		klog.V(4).ErrorS(err, "pod's annotation totalNumber illegal, gangName: %v, value: %v",
			gang.Name, pod.Annotations[extension.AnnotationGangTotalNum])
		totalChildrenNum = int64(minRequiredNumber)
	} else if totalChildrenNum != 0 && totalChildrenNum < int64(minRequiredNumber) {

		klog.V(4).Infof("pod's annotation totalNumber cannot less than minRequiredNumber, gangName: %v, totalNumber: %v,minRequiredNumber: %v",
			gang.Name, pod.Annotations[extension.AnnotationGangTotalNum], minRequiredNumber)
		totalChildrenNum = int64(minRequiredNumber)
	}
	gang.TotalChildrenNum = int(totalChildrenNum)

	mode := pod.Annotations[extension.AnnotationGangMode]
	if mode != extension.GangModeStrict && mode != extension.GangModeNonStrict {
		klog.V(4).Infof("pod's annotation GangModeAnnotation illegal, gangName: %v, value: %v",
			gang.Name, pod.Annotations[extension.AnnotationGangMode])
		mode = extension.GangModeStrict
	}
	gang.Mode = mode

	matchPolicy := util.GetGangMatchPolicyByPod(pod)
	if matchPolicy != extension.GangMatchPolicyOnlyWaiting && matchPolicy != extension.GangMatchPolicyWaitingAndRunning &&
		matchPolicy != extension.GangMatchPolicyOnceSatisfied {
		klog.V(4).Infof("pod's annotation AnnotationGangMatchPolicy illegal, gangName: %v, value: %v",
			gang.Name, matchPolicy)
		matchPolicy = extension.GangMatchPolicyOnceSatisfied
	}
	gang.GangMatchPolicy = matchPolicy

	// here we assume that Coscheduling's CreateTime equal with the pod's CreateTime
	gang.CreateTime = pod.CreationTimestamp.Time

	waitTime, err := time.ParseDuration(pod.Annotations[extension.AnnotationGangWaitTime])
	if err != nil || waitTime <= 0 {
		klog.V(4).ErrorS(err, "pod's annotation GangWaitTimeAnnotation illegal, gangName: %v, value: %v",
			gang.Name, pod.Annotations[extension.AnnotationGangWaitTime])
		waitTime = args.DefaultTimeout.Duration
	}
	gang.WaitTime = waitTime

	groupSlice, err := util.StringToGangGroupSlice(pod.Annotations[extension.AnnotationGangGroups])
	if err != nil {
		klog.V(4).ErrorS(err, "pod's annotation GangGroupsAnnotation illegal, gangName: %v, value: %v",
			gang.Name, pod.Annotations[extension.AnnotationGangGroups])
	}
	if len(groupSlice) == 0 {
		groupSlice = append(groupSlice, gang.Name)
	}
	gang.GangGroup = groupSlice
	gang.GangGroupId = util.GetGangGroupId(groupSlice)
	gang.GangFrom = GangFromPodAnnotation

	gang.HasGangInit = true

	klog.Infof("TryInitByPodConfig done, gangName: %v, minRequiredNumber: %v, totalChildrenNum: %v, "+
		"mode: %v, waitTime: %v, groupSlice: %v", gang.Name, gang.MinRequiredNumber, gang.TotalChildrenNum,
		gang.Mode, gang.WaitTime, gang.GangGroup)
	return true
}

func (gang *Gang) tryInitByPodGroup(pg *v1alpha1.PodGroup, args *config.CoschedulingArgs) {
	gang.lock.Lock()
	defer gang.lock.Unlock()
	minRequiredNumber := pg.Spec.MinMember
	gang.MinRequiredNumber = int(minRequiredNumber)

	totalChildrenNum, err := strconv.ParseInt(pg.Annotations[extension.AnnotationGangTotalNum], 10, 32)
	if err != nil {
		klog.V(4).ErrorS(err, "podGroup's annotation totalNumber illegal, gangName: %v, value: %v",
			gang.Name, pg.Annotations[extension.AnnotationGangTotalNum])
		totalChildrenNum = int64(minRequiredNumber)
	} else if totalChildrenNum != 0 && totalChildrenNum < int64(minRequiredNumber) {
		klog.V(4).Infof("podGroup's annotation totalNumber cannot less than minRequiredNumber, gangName:%v, totalNumber: %v,minRequiredNumber: %v",
			gang.Name, pg.Annotations[extension.AnnotationGangTotalNum], minRequiredNumber)
		totalChildrenNum = int64(minRequiredNumber)
	}
	gang.TotalChildrenNum = int(totalChildrenNum)

	mode := pg.Annotations[extension.AnnotationGangMode]
	if mode != extension.GangModeStrict && mode != extension.GangModeNonStrict {
		klog.V(4).Infof("podGroup's annotation GangModeAnnotation illegal, gangName: %v, value: %v",
			gang.Name, pg.Annotations[extension.AnnotationGangMode])
		mode = extension.GangModeStrict
	}
	gang.Mode = mode

	matchPolicy := pg.Annotations[extension.AnnotationGangMatchPolicy]
	if matchPolicy != extension.GangMatchPolicyOnlyWaiting && matchPolicy != extension.GangMatchPolicyWaitingAndRunning &&
		matchPolicy != extension.GangMatchPolicyOnceSatisfied {
		klog.V(4).Infof("podGroup's annotation AnnotationGangMatchPolicy illegal, gangName: %v, value: %v",
			gang.Name, matchPolicy)
		matchPolicy = extension.GangMatchPolicyOnceSatisfied
	}
	gang.GangMatchPolicy = matchPolicy

	// here we assume that Coscheduling's CreateTime equal with the podGroup CRD CreateTime
	gang.CreateTime = pg.CreationTimestamp.Time

	waitTime := util.GetWaitTimeDuration(pg, args.DefaultTimeout.Duration)
	gang.WaitTime = waitTime

	groupSlice, err := util.StringToGangGroupSlice(pg.Annotations[extension.AnnotationGangGroups])
	if err != nil {
		klog.V(4).ErrorS(err, "podGroup's annotation GangGroupsAnnotation illegal, gangName: %v, value: %v",
			gang.Name, pg.Annotations[extension.AnnotationGangGroups])
	}
	if len(groupSlice) == 0 {
		groupSlice = append(groupSlice, gang.Name)
	}
	gang.GangGroup = groupSlice
	gang.GangGroupId = util.GetGangGroupId(groupSlice)

	gang.GangFrom = GangFromPodGroupCrd

	gang.HasGangInit = true

	klog.Infof("TryInitByPodGroup done, gangName: %v, minRequiredNumber: %v, totalChildrenNum: %v, "+
		"mode: %v, waitTime: %v, groupSlice: %v", gang.Name, gang.MinRequiredNumber, gang.TotalChildrenNum,
		gang.Mode, gang.WaitTime, gang.GangGroup)
}

func (gang *Gang) SetGangGroupInfo(gangGroupInfo *GangGroupInfo) {
	gang.lock.Lock()
	defer gang.lock.Unlock()

	if !gang.GangGroupInfo.IsInitialized() {
		gang.GangGroupInfo = gangGroupInfo
		klog.Infof("SetGangGroupInfo done, gangName: %v, groupSlice: %v, gangGroupId: %v",
			gang.Name, gang.GangGroup, gang.GangGroupId)
	}
}

func (gang *Gang) deletePod(pod *v1.Pod) bool {
	if pod == nil {
		return false
	}

	gang.lock.Lock()
	defer gang.lock.Unlock()

	podId := util.GetId(pod.Namespace, pod.Name)
	klog.Infof("Delete pod from gang: %v, podName: %v", gang.Name, podId)

	delete(gang.Children, podId)
	delete(gang.PendingChildren, podId)
	gang.GangGroupInfo.DeleteIfRepresentative(pod, ReasonPodDeleted)
	delete(gang.WaitingForBindChildren, podId)
	if len(gang.WaitingForBindChildren) == 0 {
		gang.GangGroupInfo.RemoveWaitingGang(gang.Name)
	}

	delete(gang.BoundChildren, podId)
	if gang.GangFrom == GangFromPodAnnotation {
		if len(gang.Children) == 0 {
			return true
		}
	}
	return false
}

func (gang *Gang) getGangWaitTime() time.Duration {
	gang.lock.RLock()
	defer gang.lock.RUnlock()

	return gang.WaitTime
}

func (gang *Gang) getChildrenNum() int {
	gang.lock.RLock()
	defer gang.lock.RUnlock()

	return len(gang.Children)
}

func (gang *Gang) getGangMinNum() int {
	gang.lock.RLock()
	defer gang.lock.RUnlock()

	return gang.MinRequiredNumber
}

func (gang *Gang) getGangTotalNum() int {
	gang.lock.RLock()
	defer gang.lock.RUnlock()

	return gang.TotalChildrenNum
}

func (gang *Gang) getBoundPodNum() int32 {
	gang.lock.RLock()
	defer gang.lock.RUnlock()
	return int32(len(gang.BoundChildren))
}

func (gang *Gang) getGangMode() string {
	gang.lock.RLock()
	defer gang.lock.RUnlock()

	return gang.Mode
}

func (gang *Gang) getGangMatchPolicy() string {
	gang.lock.RLock()
	defer gang.lock.RUnlock()

	return gang.GangMatchPolicy
}

func (gang *Gang) getGangAssumedPods() int {
	gang.lock.RLock()
	defer gang.lock.RUnlock()

	return len(gang.WaitingForBindChildren) + len(gang.BoundChildren)
}

func (gang *Gang) getGangWaitingPods() int {
	gang.lock.RLock()
	defer gang.lock.RUnlock()

	return len(gang.WaitingForBindChildren)
}

func (gang *Gang) getCreateTime() time.Time {
	gang.lock.RLock()
	defer gang.lock.RUnlock()

	return gang.CreateTime
}

func (gang *Gang) getGangGroup() []string {
	gang.lock.RLock()
	defer gang.lock.RUnlock()

	return gang.GangGroup
}

func (gang *Gang) isGangOnceResourceSatisfied() bool {
	gang.lock.RLock()
	defer gang.lock.RUnlock()

	return gang.GangGroupInfo.isGangOnceResourceSatisfied()
}

func (gang *Gang) setChild(pod *v1.Pod) {
	gang.lock.Lock()
	defer gang.lock.Unlock()

	podId := util.GetId(pod.Namespace, pod.Name)
	gang.Children[podId] = pod
	if _, ok := gang.Children[podId]; !ok {
		klog.V(6).Infof("SetChild, gangName: %v, childName: %v", gang.Name, podId)
	} else {
		klog.V(6).Infof("UpdateChild, gangName: %v, childName: %v", gang.Name, podId)
	}
	if pod.Spec.NodeName == "" && gang.WaitingForBindChildren[podId] == nil {
		gang.PendingChildren[podId] = pod
		if _, ok := gang.PendingChildren[podId]; !ok {
			klog.Infof("SetPendingChild, gangName: %v, childName: %v", gang.Name, podId)
		} else {
			klog.Infof("UpdatePendingChild, gangName: %v, childName: %v", gang.Name, podId)
		}
	}
}

func (gang *Gang) addAssumedPod(pod *v1.Pod) {
	gang.lock.Lock()
	defer gang.lock.Unlock()

	podId := util.GetId(pod.Namespace, pod.Name)
	if _, ok := gang.WaitingForBindChildren[podId]; !ok {
		gang.WaitingForBindChildren[podId] = pod
		klog.Infof("AddAssumedPod, gangName: %v, podName: %v", gang.Name, podId)
	}
	delete(gang.PendingChildren, podId)
}

func (gang *Gang) delAssumedPod(pod *v1.Pod) {
	gang.lock.Lock()
	defer gang.lock.Unlock()

	podId := util.GetId(pod.Namespace, pod.Name)
	if _, ok := gang.WaitingForBindChildren[podId]; ok {
		delete(gang.WaitingForBindChildren, podId)
		if pendingPod := gang.Children[podId]; pendingPod != nil {
			gang.PendingChildren[podId] = pendingPod
		}
		if len(gang.WaitingForBindChildren) == 0 {
			gang.GangGroupInfo.RemoveWaitingGang(gang.Name)
		}
		klog.Infof("delAssumedPod, gangName: %v, podName: %v", gang.Name, podId)
	}
}

func (gang *Gang) getChildrenFromGang() (children []*v1.Pod) {
	gang.lock.RLock()
	defer gang.lock.RUnlock()
	children = make([]*v1.Pod, 0)
	for _, pod := range gang.Children {
		children = append(children, pod)
	}
	return
}

func (gang *Gang) getPendingChildrenFromGang() (children []*v1.Pod) {
	gang.lock.RLock()
	defer gang.lock.RUnlock()
	children = make([]*v1.Pod, 0)
	for _, pod := range gang.PendingChildren {
		children = append(children, pod)
	}
	return
}

func (gang *Gang) isGangFromAnnotation() bool {
	gang.lock.RLock()
	defer gang.lock.RUnlock()
	return gang.GangFrom == GangFromPodAnnotation
}

func (gang *Gang) setResourceSatisfied() {
	gang.lock.Lock()
	defer gang.lock.Unlock()

	gang.GangGroupInfo.setResourceSatisfied()
}

func (gang *Gang) addBoundPod(pod *v1.Pod) {
	gang.lock.Lock()
	defer gang.lock.Unlock()

	podId := util.GetId(pod.Namespace, pod.Name)
	delete(gang.WaitingForBindChildren, podId)
	if len(gang.WaitingForBindChildren) == 0 {
		gang.GangGroupInfo.RemoveWaitingGang(gang.Name)
	}
	delete(gang.PendingChildren, podId)
	gang.GangGroupInfo.DeleteIfRepresentative(pod, ReasonPodBound)
	gang.BoundChildren[podId] = pod

	klog.Infof("AddBoundPod, gangName: %v, podName: %v", gang.Name, podId)
	if !gang.GangGroupInfo.isGangOnceResourceSatisfied() {
		gang.GangGroupInfo.setResourceSatisfied()
		klog.Infof("Gang ResourceSatisfied due to addBoundPod, gangName: %v", gang.Name)
	}
}

func (gang *Gang) addWaitingGang() {
	gang.lock.Lock()
	defer gang.lock.Unlock()
	gang.GangGroupInfo.AddWaitingGang()
}

func (gang *Gang) clearWaitingGang() {
	gang.lock.Lock()
	defer gang.lock.Unlock()
	gang.GangGroupInfo.ClearWaitingGang()
}

func (gang *Gang) removeWaitingGang() {
	gang.lock.Lock()
	defer gang.lock.Unlock()
	gang.GangGroupInfo.RemoveWaitingGang(gang.Name)
}

func (gang *Gang) isGangWorthRequeue() bool {
	gang.lock.RLock()
	defer gang.lock.RUnlock()
	return gang.HasGangInit && len(gang.Children) >= gang.MinRequiredNumber
}

func (gang *Gang) pickSomeChildren() *v1.Pod {
	gang.lock.RLock()
	defer gang.lock.RUnlock()
	for _, pod := range gang.PendingChildren {
		return pod
	}
	return nil
}

func (gang *Gang) isGangValidForPermit() bool {
	gang.lock.RLock()
	defer gang.lock.RUnlock()
	if !gang.HasGangInit {
		klog.Infof("isGangValidForPermit find gang hasn't inited ,gang: %v", gang.Name)
		return false
	}

	switch gang.GangMatchPolicy {
	case extension.GangMatchPolicyOnlyWaiting:
		return len(gang.WaitingForBindChildren) >= gang.MinRequiredNumber
	case extension.GangMatchPolicyWaitingAndRunning:
		return len(gang.WaitingForBindChildren)+len(gang.BoundChildren) >= gang.MinRequiredNumber
	default:
		return len(gang.WaitingForBindChildren) >= gang.MinRequiredNumber || gang.GangGroupInfo.isGangOnceResourceSatisfied()
	}
}

func (gang *Gang) RecordIfNoRepresentatives(pod *v1.Pod) error {
	gang.lock.Lock()
	defer gang.lock.Unlock()
	podKey := util.GetId(pod.Namespace, pod.Name)
	if gang.PendingChildren[podKey] == nil {
		// avoid pod is not exists in gang cache, resulting representativePodKey leak
		return fmt.Errorf(ErrPodIsNotExistsInGangCache, podKey)
	}

	representativePodKey := gang.GangGroupInfo.RecordIfNoRepresentatives(pod)
	if representativePodKey != podKey {
		return fmt.Errorf(ErrRepresentativePodAlreadyExists, representativePodKey, gang.GangGroupId)
	}
	return nil
}

func (gang *Gang) ClearCurrentRepresentative(reason string) {
	gang.lock.Lock()
	defer gang.lock.Unlock()
	gang.GangGroupInfo.ClearCurrentRepresentative(reason)
}
