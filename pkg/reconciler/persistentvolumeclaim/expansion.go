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

	"github.com/cloudnative-pg/machinery/pkg/log"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	resourcestatus "github.com/cloudnative-pg/cloudnative-pg/pkg/resources/status"
)

// maxListedRefusals is the number of refused PVCs named in the condition
// message, the others are counted
const maxListedRefusals = 10

// volumeExpansionRefusal is the answer of the API server refusing to expand
// a PVC to the size requested by the cluster
type volumeExpansionRefusal struct {
	pvcName      string
	storageClass string
	requested    resource.Quantity
	err          error
}

func newVolumeExpansionRefusal(
	pvc *corev1.PersistentVolumeClaim,
	requested resource.Quantity,
	err error,
) *volumeExpansionRefusal {
	return &volumeExpansionRefusal{
		pvcName:      pvc.Name,
		storageClass: ptr.Deref(pvc.Spec.StorageClassName, ""),
		requested:    requested,
		err:          err,
	}
}

func (r *volumeExpansionRefusal) Error() string {
	return fmt.Sprintf("error while changing PVC storage requirement: %s", r.err)
}

func (r *volumeExpansionRefusal) Unwrap() error {
	return r.err
}

// reason classifies the refusal from the answer of the API server, which
// carries no structured cause: the messages matched here are the ones of the
// PVC resize admission and of the ResourceQuota admission
func (r *volumeExpansionRefusal) reason() apiv1.ConditionReason {
	message := r.err.Error()
	switch {
	case strings.Contains(message, "only dynamically provisioned pvc can be resized"):
		return apiv1.ConditionReasonStorageClassForbidsExpansion
	case strings.Contains(message, "exceeded quota"), strings.Contains(message, "failed quota"):
		return apiv1.ConditionReasonQuotaExceeded
	default:
		return apiv1.ConditionReasonExpansionForbidden
	}
}

// remedy is the advice given in the condition message for a reason
func remedy(reason apiv1.ConditionReason) string {
	const keepSize = "or set resizeInUseVolumes to false to keep the existing volumes at their size; " +
		"the storage size of the cluster cannot be lowered back"
	switch reason {
	case apiv1.ConditionReasonStorageClassForbidsExpansion:
		return "Set allowVolumeExpansion to true on the StorageClass, " + keepSize
	case apiv1.ConditionReasonQuotaExceeded:
		return "Raise the requests.storage ResourceQuota of the namespace, " + keepSize
	default:
		return "Check what refused the request, " + keepSize
	}
}

// volumeExpansionRefusedCondition builds the condition reporting the
// refusals, sorted by PVC name: the reason and the answer quoted are the ones
// of the first refused PVC
func volumeExpansionRefusedCondition(refusals []*volumeExpansionRefusal) metav1.Condition {
	slices.SortFunc(refusals, func(a, b *volumeExpansionRefusal) int {
		return strings.Compare(a.pvcName, b.pvcName)
	})

	var message strings.Builder
	fmt.Fprintf(&message, "The API server refused to expand %d PVC(s) to the size requested by the cluster: ",
		len(refusals))
	for idx, refusal := range refusals {
		if idx == maxListedRefusals {
			fmt.Fprintf(&message, " and %d more", len(refusals)-maxListedRefusals)
			break
		}
		if idx > 0 {
			message.WriteString(", ")
		}
		storageClass := "the default StorageClass"
		if refusal.storageClass != "" {
			storageClass = fmt.Sprintf("StorageClass %q", refusal.storageClass)
		}
		fmt.Fprintf(&message, "%s to %s (%s)", refusal.pvcName, refusal.requested.String(), storageClass)
	}

	first := refusals[0]
	reason := first.reason()
	fmt.Fprintf(&message, ". Answer for %s: %s. %s.", first.pvcName, first.err.Error(), remedy(reason))

	return metav1.Condition{
		Type:    string(apiv1.ConditionVolumeExpansionRefused),
		Status:  metav1.ConditionTrue,
		Reason:  string(reason),
		Message: message.String(),
	}
}

// reportVolumeExpansion sets the VolumeExpansionRefused condition from the
// refusals of this reconciliation, and records an Event when it changes. A
// cluster that never had a refusal gets no condition.
func reportVolumeExpansion(
	ctx context.Context,
	c client.Client,
	recorder record.EventRecorder,
	cluster *apiv1.Cluster,
	refusals []*volumeExpansionRefusal,
) error {
	existing := meta.FindStatusCondition(cluster.Status.Conditions, string(apiv1.ConditionVolumeExpansionRefused))

	var condition metav1.Condition
	switch {
	case len(refusals) > 0:
		condition = volumeExpansionRefusedCondition(refusals)
	case existing == nil || existing.Status == metav1.ConditionFalse:
		return nil
	default:
		condition = metav1.Condition{
			Type:    string(apiv1.ConditionVolumeExpansionRefused),
			Status:  metav1.ConditionFalse,
			Reason:  string(apiv1.ConditionReasonVolumeExpansionAccepted),
			Message: "Every PVC accepts the size requested by the cluster",
		}
	}
	condition.ObservedGeneration = cluster.Generation

	changed := existing == nil ||
		existing.Status != condition.Status ||
		existing.Reason != condition.Reason ||
		existing.Message != condition.Message

	if err := resourcestatus.PatchConditionsWithOptimisticLock(ctx, c, cluster, condition); err != nil {
		return err
	}

	if !changed {
		return nil
	}

	contextLogger := log.FromContext(ctx)
	if condition.Status == metav1.ConditionTrue {
		contextLogger.Warning("Volume expansion refused", "reason", condition.Reason, "message", condition.Message)
		recorder.Event(cluster, corev1.EventTypeWarning, condition.Reason, condition.Message)
	} else {
		contextLogger.Info("Volume expansion accepted again")
		recorder.Event(cluster, corev1.EventTypeNormal, condition.Reason, condition.Message)
	}

	return nil
}
