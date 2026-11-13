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
// not terminate because it has no disk space for WALs, or because another
// volume is full, see GetMissingDiskSpaceTermination
func IsWALSpaceAvailableOnPod(pod *corev1.Pod) bool {
	// Return true if WAL space IS available (i.e., NOT terminated for missing disk space)
	return GetMissingDiskSpaceTermination(pod) == nil
}

// GetMissingDiskSpaceTermination returns the termination of the PostgreSQL
// container of a Pod for a lack of disk space, with the
// MissingWALDiskSpaceExitCode exit code: the current one, or the last one
// while the container is not ready again. It returns nil when there is none.
func GetMissingDiskSpaceTermination(pod *corev1.Pod) *corev1.ContainerStateTerminated {
	isTerminatedForMissingDiskSpace := func(state *corev1.ContainerState) bool {
		return state.Terminated != nil && state.Terminated.ExitCode == apiv1.MissingWALDiskSpaceExitCode
	}

	pgContainerStatus := getPostgresContainerStatus(pod)
	switch {
	case pgContainerStatus == nil:
		return nil
	case isTerminatedForMissingDiskSpace(&pgContainerStatus.State):
		return pgContainerStatus.State.Terminated
	case !pgContainerStatus.Ready && isTerminatedForMissingDiskSpace(&pgContainerStatus.LastTerminationState):
		return pgContainerStatus.LastTerminationState.Terminated
	default:
		return nil
	}
}

// HasPostgresContainerTerminationReason tells whether the PostgreSQL container
// of a Pod is terminated, or was terminated and is not ready yet, for the
// given reason
func HasPostgresContainerTerminationReason(pod *corev1.Pod, reason func(state *corev1.ContainerState) bool) bool {
	pgContainerStatus := getPostgresContainerStatus(pod)

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

// getPostgresContainerStatus returns the status of the PostgreSQL container
// of a Pod, nil when the Pod has none
func getPostgresContainerStatus(pod *corev1.Pod) *corev1.ContainerStatus {
	for i := range pod.Status.ContainerStatuses {
		if pod.Status.ContainerStatuses[i].Name == PostgresContainerName {
			return &pod.Status.ContainerStatuses[i]
		}
	}
	return nil
}
