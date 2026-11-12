/*
Copyright © contributors to CloudNativePG, established as
CloudNativePG a Series of LF Projects, LLC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

SPDX-License-Identifier: Apache-2.0
*/

package utils

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Pod conditions test suite", func() {
	Describe("Must check for Running PODs", func() {
		It("Detect PODs without conditions are not running", func() {
			pod := corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{},
				},
			}
			Expect(IsPodReady(pod)).To(BeFalse())
		})

		It("Detects Ready PODs as running", func() {
			pod := corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			}
			Expect(IsPodReady(pod)).To(BeTrue())
		})

		It("Detects not ready PODs are not running", func() {
			pod := corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodReady,
							Status: corev1.ConditionFalse,
						},
					},
				},
			}
			Expect(IsPodReady(pod)).To(BeFalse())
		})

		It("Returns false when ContainersReady is True but PodReady is False", func() {
			pod := corev1.Pod{
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.ContainersReady,
							Status: corev1.ConditionTrue,
						},
						{
							Type:   corev1.PodReady,
							Status: corev1.ConditionFalse,
						},
					},
				},
			}
			Expect(IsPodReady(pod)).To(BeFalse())
		})

		It("return 0 if the list of Pods is empty", func() {
			var list []corev1.Pod
			Expect(CountReadyPods(list)).To(Equal(0))
		})

		It("return the number of ready Pods", func() {
			car1 := corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "car-1",
					Namespace: "default",
				},
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			}

			car2 := corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "car-2",
					Namespace: "default",
				},
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			}

			foo := corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "default",
				},
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodReady,
							Status: corev1.ConditionFalse,
						},
					},
				},
			}

			bar := corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "bar",
					Namespace: "default",
				},
				Status: corev1.PodStatus{
					Conditions: []corev1.PodCondition{
						{
							Type:   corev1.PodReady,
							Status: corev1.ConditionFalse,
						},
					},
				},
			}
			podList := []corev1.Pod{car2, foo, bar, car1}

			Expect(CountReadyPods(podList)).To(Equal(2))
		})
	})

	Describe("Must detect if a pod is unschedulable", func() {
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.PodScheduled,
						Status: corev1.ConditionFalse,
						Reason: corev1.PodReasonUnschedulable,
					},
				},
			},
		}
		Expect(IsPodUnschedulable(pod)).To(BeTrue())

		pod = &corev1.Pod{
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.PodReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		}
		Expect(IsPodUnschedulable(pod)).To(BeFalse())
	})

	DescribeTable(
		"IsPodStuckTerminating",
		func(deletionTime *metav1.Time, expected bool) {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: deletionTime},
			}
			Expect(IsPodStuckTerminating(pod)).To(Equal(expected))
		},
		Entry("no deletion timestamp", nil, false),
		Entry("deletion deadline in the past", &metav1.Time{Time: time.Now().Add(-time.Minute)}, true),
		Entry("deletion deadline in the future", &metav1.Time{Time: time.Now().Add(time.Hour)}, false),
	)
})

// The reason of an inactive Pod comes from the Pod itself, so that the owner
// of a Cluster sees it on the Cluster instead of having to describe the Pod
var _ = DescribeTable("PodInactiveReason",
	func(pod corev1.Pod, expected string) {
		Expect(PodInactiveReason(pod)).To(Equal(expected))
	},
	Entry("a running Pod has no reason", corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodRunning}}, ""),
	Entry("a Pod being deleted", corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &metav1.Time{Time: time.Now()}},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}, "being deleted"),
	Entry("a failed Pod, with the kubelet's reason", corev1.Pod{
		Status: corev1.PodStatus{
			Phase: corev1.PodFailed, Reason: "Evicted", Message: "The node was low on resource: ephemeral-storage.",
		},
	}, "Failed (Evicted: The node was low on resource: ephemeral-storage.)"),
	Entry("a Pod the scheduler cannot place", corev1.Pod{
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{{
				Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: corev1.PodReasonUnschedulable,
				Message: "0/1 nodes are available: pod has unbound immediate PersistentVolumeClaims.",
			}},
		},
	}, "not scheduled (Unschedulable: 0/1 nodes are available: pod has unbound immediate PersistentVolumeClaims.)"),
	Entry("a Pod whose init container cannot start, before the app containers", corev1.Pod{
		Status: corev1.PodStatus{
			Phase:      corev1.PodPending,
			Conditions: []corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionTrue}},
			InitContainerStatuses: []corev1.ContainerStatus{{
				Name: "bootstrap-controller",
				State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
					Reason: "ImagePullBackOff", Message: "Back-off pulling image \"ghcr.io/x/y:z\"",
				}},
			}},
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:  "postgres",
				State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "PodInitializing"}},
			}},
		},
	}, "container bootstrap-controller waiting (ImagePullBackOff: Back-off pulling image \"ghcr.io/x/y:z\")"),
	Entry("a Pod whose app container waits for a volume", corev1.Pod{
		Status: corev1.PodStatus{
			Phase:      corev1.PodPending,
			Conditions: []corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionTrue}},
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:  "postgres",
				State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ContainerCreating"}},
			}},
		},
	}, "container postgres waiting (ContainerCreating)"),
	Entry("a Pending Pod reporting nothing yet", corev1.Pod{Status: corev1.PodStatus{Phase: corev1.PodPending}}, ""),
)
