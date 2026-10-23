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

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
)

// IsWALSpaceAvailableOnPod checks that the PostgreSQL container of a Pod did
// not terminate because it has no disk space for WALs
func IsWALSpaceAvailableOnPod(pod *corev1.Pod) bool {
	isTerminatedForMissingWALDiskSpace := func(state *corev1.ContainerState) bool {
		return state.Terminated != nil && state.Terminated.ExitCode == apiv1.MissingWALDiskSpaceExitCode
	}
	// Return true if WAL space IS available (i.e., NOT terminated for missing disk space)
	return !HasPostgresContainerTerminationReason(pod, isTerminatedForMissingWALDiskSpace)
}

// HasPostgresContainerTerminationReason tells whether the PostgreSQL container
// of a Pod is terminated, or was terminated and is not ready yet, for the
// given reason
func HasPostgresContainerTerminationReason(pod *corev1.Pod, reason func(state *corev1.ContainerState) bool) bool {
	var pgContainerStatus *corev1.ContainerStatus
	for i := range pod.Status.ContainerStatuses {
		status := pod.Status.ContainerStatuses[i]
		if status.Name == PostgresContainerName {
			pgContainerStatus = &pod.Status.ContainerStatuses[i]
			break
		}
	}

	// This is not an instance Pod as there's no PostgreSQL
	// container
	if pgContainerStatus == nil {
		return false
	}

	// If the Pod was terminated with the specified reason,
	// then return true
	if reason(&pgContainerStatus.State) {
		return true
	}

	// The container is not ready and was last terminated with the specified reason.
	// Return true to indicate the termination reason was found
	if !pgContainerStatus.Ready && reason(&pgContainerStatus.LastTerminationState) {
		return true
	}

	return false
}
