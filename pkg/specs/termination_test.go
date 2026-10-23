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

package specs

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Pod termination reason detection", func() {
	It("should detect when a pod has no PostgreSQL container", func() {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: "other-container"},
				},
			},
		}

		// When no postgres container exists, HasPostgresContainerTerminationReason returns false
		result := HasPostgresContainerTerminationReason(pod, func(state *corev1.ContainerState) bool {
			return state.Terminated != nil
		})
		Expect(result).To(BeFalse())
	})

	It("should detect termination with specific exit code in current state", func() {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name: "postgres",
						State: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{
								ExitCode: apiv1.MissingWALDiskSpaceExitCode,
							},
						},
					},
				},
			},
		}

		result := HasPostgresContainerTerminationReason(pod, func(state *corev1.ContainerState) bool {
			return state.Terminated != nil && state.Terminated.ExitCode == apiv1.MissingWALDiskSpaceExitCode
		})
		Expect(result).To(BeTrue())
	})

	It("should return false when termination reason does not match", func() {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name: "postgres",
						State: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{
								ExitCode: 1, // Different exit code
							},
						},
					},
				},
			},
		}

		result := HasPostgresContainerTerminationReason(pod, func(state *corev1.ContainerState) bool {
			return state.Terminated != nil && state.Terminated.ExitCode == apiv1.MissingWALDiskSpaceExitCode
		})
		Expect(result).To(BeFalse())
	})

	It("should return false when container is ready despite last termination", func() {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name: "postgres",
						State: corev1.ContainerState{
							Running: &corev1.ContainerStateRunning{},
						},
						Ready: true,
						LastTerminationState: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{
								ExitCode: apiv1.MissingWALDiskSpaceExitCode,
							},
						},
					},
				},
			},
		}

		result := HasPostgresContainerTerminationReason(pod, func(state *corev1.ContainerState) bool {
			return state.Terminated != nil && state.Terminated.ExitCode == apiv1.MissingWALDiskSpaceExitCode
		})
		Expect(result).To(BeFalse())
	})

	It("IsWALSpaceAvailableOnPod should return true when space is available", func() {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name: "postgres",
						State: corev1.ContainerState{
							Running: &corev1.ContainerStateRunning{},
						},
						Ready: true,
					},
				},
			},
		}

		Expect(IsWALSpaceAvailableOnPod(pod)).To(BeTrue())
	})

	It("IsWALSpaceAvailableOnPod should return false when terminated due to missing disk space", func() {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name: "postgres",
						State: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{
								ExitCode: apiv1.MissingWALDiskSpaceExitCode,
							},
						},
					},
				},
			},
		}

		Expect(IsWALSpaceAvailableOnPod(pod)).To(BeFalse())
	})
})
