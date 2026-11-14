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

package persistentvolumeclaim

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"
)

// FileSystemResizeGracePeriod is how long the filesystem of a PVC may wait
// for its resize before the Pod is said to need a restart. With a storage
// class resizing online, the kubelet resizes it in a few seconds
const FileSystemResizeGracePeriod = 2 * time.Minute

// now is the clock, replaced by the tests
var now = time.Now

// fileSystemResizePendingSince returns since when the filesystem of a PVC
// waits to be resized on the node, and whether it does
func fileSystemResizePendingSince(pvc *corev1.PersistentVolumeClaim) (time.Time, bool) {
	for _, condition := range pvc.Status.Conditions {
		if condition.Type == corev1.PersistentVolumeClaimFileSystemResizePending &&
			condition.Status == corev1.ConditionTrue {
			return condition.LastTransitionTime.Time, true
		}
	}
	return time.Time{}, false
}

// IsFileSystemResizeAwaited tells whether the filesystem of a PVC waits for
// its resize within the grace period: the cluster is reconciled again once
// it is over, to tell whether a Pod needs a restart
func IsFileSystemResizeAwaited(pvcs []corev1.PersistentVolumeClaim) bool {
	return slices.ContainsFunc(pvcs, func(pvc corev1.PersistentVolumeClaim) bool {
		since, pending := fileSystemResizePendingSince(&pvc)
		return pending && now().Sub(since) < FileSystemResizeGracePeriod
	})
}

// getInstancesToRestartForResize returns the instances whose PVCs wait for
// the resize of their filesystem beyond the grace period, sorted: their
// storage class does not resize a volume in use, and the Pod must be
// restarted for the new size to be available
func getInstancesToRestartForResize(pvcs []corev1.PersistentVolumeClaim) []string {
	var instances []string
	for idx := range pvcs {
		since, pending := fileSystemResizePendingSince(&pvcs[idx])
		if !pending || now().Sub(since) < FileSystemResizeGracePeriod {
			continue
		}
		if instance := pvcs[idx].Labels[utils.InstanceNameLabelName]; instance != "" {
			instances = append(instances, instance)
		}
	}
	slices.Sort(instances)
	return slices.Compact(instances)
}

// reportPendingFileSystemResize sets the VolumeResizePending condition from
// the PVCs whose filesystem waits for a restart of their Pod, and records an
// Event when it changes. A cluster that never waited gets no condition.
func reportPendingFileSystemResize(
	ctx context.Context,
	c client.Client,
	recorder record.EventRecorder,
	cluster *apiv1.Cluster,
	pvcs []corev1.PersistentVolumeClaim,
) error {
	existing := meta.FindStatusCondition(cluster.Status.Conditions, string(apiv1.ConditionVolumeResizePending))
	instances := getInstancesToRestartForResize(pvcs)

	var condition metav1.Condition
	switch {
	case len(instances) > 0:
		condition = metav1.Condition{
			Type:   string(apiv1.ConditionVolumeResizePending),
			Status: metav1.ConditionTrue,
			Reason: string(apiv1.ConditionReasonFileSystemResizePending),
			Message: fmt.Sprintf("The volumes of %s were resized, and their filesystem is resized when the Pod "+
				"restarts: the storage class does not resize a volume in use. Restart these instances one at a "+
				"time, the replicas first", strings.Join(instances, ", ")),
		}
	case existing == nil || existing.Status == metav1.ConditionFalse:
		return nil
	default:
		condition = metav1.Condition{
			Type:    string(apiv1.ConditionVolumeResizePending),
			Status:  metav1.ConditionFalse,
			Reason:  string(apiv1.ConditionReasonVolumeResizeCompleted),
			Message: "No volume waits for a restart of its Pod to be resized",
		}
	}
	return setReportedCondition(ctx, c, recorder, cluster, existing, condition)
}
