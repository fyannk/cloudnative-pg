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

	"github.com/cloudnative-pg/machinery/pkg/log"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/postgres"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/specs"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"
)

// AutoGrowCheckInterval is how often a cluster with autoGrow is reconciled
// again, to read the usage of its volumes
const AutoGrowCheckInterval = 30 * time.Second

// HasAutoGrow tells whether a volume of the cluster has autoGrow
func HasAutoGrow(cluster *apiv1.Cluster) bool {
	return len(getAutoGrowVolumes(cluster)) > 0
}

// pvcGroup identifies the PVCs of a volume across the instances: their role
// and, for a tablespace volume, the name of the tablespace
type pvcGroup struct {
	role       utils.PVCRole
	tablespace string
}

func pvcGroupFromLabels(labels map[string]string) pvcGroup {
	group := pvcGroup{role: utils.PVCRole(labels[utils.PvcRoleLabelName])}
	if group.role == utils.PVCRolePgTablespace {
		group.tablespace = labels[utils.TablespaceNameLabelName]
	}
	return group
}

// volumeName is the name of the volume in the usages reported by the
// instances
func (group pvcGroup) volumeName() string {
	switch group.role {
	case utils.PVCRolePgWal:
		return postgres.WalVolumeName
	case utils.PVCRolePgTablespace:
		return postgres.TablespaceVolumePrefix + group.tablespace
	default:
		return postgres.PgDataVolumeName
	}
}

func (group pvcGroup) String() string {
	switch group.role {
	case utils.PVCRolePgWal:
		return "WAL"
	case utils.PVCRolePgTablespace:
		return fmt.Sprintf("tablespace %s", group.tablespace)
	default:
		return "PGDATA"
	}
}

// holdsWAL tells whether the volume holds the WAL files of the cluster
func (group pvcGroup) holdsWAL(cluster *apiv1.Cluster) bool {
	switch group.role {
	case utils.PVCRolePgWal:
		return true
	case utils.PVCRolePgData:
		return !cluster.ShouldCreateWalArchiveVolume()
	default:
		return false
	}
}

// getLargestRequests returns the largest storage request among the PVCs of
// each volume, terminating PVCs excluded. The PVCs are the record of the
// size a volume was grown to, outside the cluster specification.
func getLargestRequests(pvcs []corev1.PersistentVolumeClaim) map[pvcGroup]resource.Quantity {
	result := make(map[pvcGroup]resource.Quantity)
	for idx := range pvcs {
		pvc := &pvcs[idx]
		if pvc.DeletionTimestamp != nil {
			continue
		}
		group := pvcGroupFromLabels(pvc.Labels)
		request := pvc.Spec.Resources.Requests.Storage()
		if largest, ok := result[group]; !ok || request.Cmp(largest) > 0 {
			result[group] = request.DeepCopy()
		}
	}
	return result
}

// withGroupSize returns the storage configuration of a volume, with the size
// raised to the largest request of its existing PVCs: a PVC created for a
// volume that was grown, by autoGrow or outside the cluster, must be able to
// hold the data of the other instances
func withGroupSize(
	configuration apiv1.StorageConfiguration,
	group pvcGroup,
	largestRequests map[pvcGroup]resource.Quantity,
) apiv1.StorageConfiguration {
	largest, ok := largestRequests[group]
	if !ok {
		return configuration
	}
	if size := configuration.GetSizeOrNil(); size != nil && size.Cmp(largest) >= 0 {
		return configuration
	}
	configuration.Size = largest.String()
	return configuration
}

// autoGrowStep is the growth of the PVCs of a volume
type autoGrowStep struct {
	group       pvcGroup
	from        resource.Quantity
	to          resource.Quantity
	instance    string
	usedPercent float64
}

// autoGrowStop is a volume above its threshold that is not grown
type autoGrowStop struct {
	group     pvcGroup
	reason    apiv1.ConditionReason
	size      resource.Quantity
	threshold int
}

// autoGrowPlan is what autoGrow decided for the volumes of a cluster
type autoGrowPlan struct {
	// sizes is the size requested to the existing PVCs of each volume with
	// autoGrow, never below their largest request
	sizes map[pvcGroup]resource.Quantity
	steps []autoGrowStep
	stops []autoGrowStop
}

// autoGrowVolume is a volume of the cluster with autoGrow
type autoGrowVolume struct {
	group         pvcGroup
	configuration *apiv1.StorageConfiguration
}

func getAutoGrowVolumes(cluster *apiv1.Cluster) []autoGrowVolume {
	var volumes []autoGrowVolume
	if cluster.Spec.StorageConfiguration.AutoGrow != nil {
		volumes = append(volumes, autoGrowVolume{
			group:         pvcGroup{role: utils.PVCRolePgData},
			configuration: &cluster.Spec.StorageConfiguration,
		})
	}
	if cluster.ShouldCreateWalArchiveVolume() && cluster.Spec.WalStorage.AutoGrow != nil {
		volumes = append(volumes, autoGrowVolume{
			group:         pvcGroup{role: utils.PVCRolePgWal},
			configuration: cluster.Spec.WalStorage,
		})
	}
	for idx := range cluster.Spec.Tablespaces {
		tablespace := &cluster.Spec.Tablespaces[idx]
		if tablespace.Storage.AutoGrow != nil {
			volumes = append(volumes, autoGrowVolume{
				group:         pvcGroup{role: utils.PVCRolePgTablespace, tablespace: tablespace.Name},
				configuration: &tablespace.Storage,
			})
		}
	}
	return volumes
}

// hasPendingExpansion tells whether a PVC has not got the size it requests
// yet
func hasPendingExpansion(pvc *corev1.PersistentVolumeClaim) bool {
	if isResizing(*pvc) || hasPVCCondition(*pvc, corev1.PersistentVolumeClaimFileSystemResizePending) {
		return true
	}
	capacity, ok := pvc.Status.Capacity[corev1.ResourceStorage]
	return !ok || capacity.Cmp(*pvc.Spec.Resources.Requests.Storage()) < 0
}

// isUsageOfVolume tells whether the filesystem reported by an instance is
// the one of its PVC. A provisioner backing the volumes with a directory,
// such as local-path or NFS, reports the filesystem holding the directory,
// which is not grown with the PVC.
func isUsageOfVolume(usage postgres.VolumeUsage, pvc *corev1.PersistentVolumeClaim) bool {
	capacity, ok := pvc.Status.Capacity[corev1.ResourceStorage]
	if !ok || capacity.Sign() <= 0 {
		return false
	}
	// the filesystem takes some space from the device for its metadata,
	// and providers may round the capacity up
	total := float64(usage.TotalBytes)
	return total >= capacity.AsApproximateFloat64()/2 && total <= capacity.AsApproximateFloat64()*1.1
}

// getUsedPercent returns the most used filesystem of a volume among the
// instances, and the instance reporting it. An instance whose PostgreSQL
// stopped because the WAL volume is full counts as 100% used for the volume
// holding the WAL files: it cannot report its usage anymore.
func getUsedPercent(
	cluster *apiv1.Cluster,
	group pvcGroup,
	pvcs []*corev1.PersistentVolumeClaim,
	instances postgres.PostgresqlStatusList,
) (usedPercent float64, instance string) {
	pvcOfInstance := make(map[string]*corev1.PersistentVolumeClaim, len(pvcs))
	for _, pvc := range pvcs {
		pvcOfInstance[pvc.Labels[utils.InstanceNameLabelName]] = pvc
	}

	for _, status := range instances.Items {
		if status.Pod == nil {
			continue
		}
		pvc := pvcOfInstance[status.Pod.Name]
		if pvc == nil {
			continue
		}

		if group.holdsWAL(cluster) && !specs.IsWALSpaceAvailableOnPod(status.Pod) {
			return 100, status.Pod.Name
		}
		if status.Error != nil {
			continue
		}
		for _, usage := range status.VolumeUsages {
			if usage.Volume != group.volumeName() || !isUsageOfVolume(usage, pvc) {
				continue
			}
			if used := usage.UsedPercent(); used > usedPercent {
				usedPercent, instance = used, status.Pod.Name
			}
		}
	}

	return usedPercent, instance
}

// planAutoGrow decides the size of the PVCs of each volume with autoGrow.
// The PVCs of a volume are grown together, by one step, when the most used
// of their filesystems crosses the threshold, and only once every PVC of the
// volume got the size of the previous step.
func planAutoGrow(
	cluster *apiv1.Cluster,
	pvcs []corev1.PersistentVolumeClaim,
	instances postgres.PostgresqlStatusList,
) (autoGrowPlan, error) {
	plan := autoGrowPlan{sizes: make(map[pvcGroup]resource.Quantity)}
	volumes := getAutoGrowVolumes(cluster)
	if len(volumes) == 0 {
		return plan, nil
	}

	largestRequests := getLargestRequests(pvcs)
	archivingFails := meta.IsStatusConditionFalse(
		cluster.Status.Conditions, string(apiv1.ConditionContinuousArchiving))

	for _, volume := range volumes {
		var groupPVCs []*corev1.PersistentVolumeClaim
		for idx := range pvcs {
			if pvcs[idx].DeletionTimestamp == nil && pvcGroupFromLabels(pvcs[idx].Labels) == volume.group {
				groupPVCs = append(groupPVCs, &pvcs[idx])
			}
		}
		configuration := withGroupSize(*volume.configuration, volume.group, largestRequests)
		size := configuration.GetSizeOrNil()
		if len(groupPVCs) == 0 || size == nil {
			continue
		}
		plan.sizes[volume.group] = *size

		if slices.ContainsFunc(groupPVCs, hasPendingExpansion) {
			continue
		}

		autoGrow := volume.configuration.AutoGrow
		usedPercent, instance := getUsedPercent(cluster, volume.group, groupPVCs, instances)
		if usedPercent < float64(autoGrow.GetThreshold()) {
			continue
		}

		maxSize, err := autoGrow.GetMaxSize()
		if err != nil {
			return autoGrowPlan{}, fmt.Errorf("invalid autoGrow maxSize of the %s volume: %w", volume.group, err)
		}
		stop := autoGrowStop{group: volume.group, size: *size, threshold: autoGrow.GetThreshold()}
		switch {
		case volume.group.holdsWAL(cluster) && archivingFails:
			stop.reason = apiv1.ConditionReasonWALArchivingFailing
			plan.stops = append(plan.stops, stop)
			continue
		case size.Cmp(maxSize) >= 0:
			stop.reason = apiv1.ConditionReasonMaxSizeReached
			plan.stops = append(plan.stops, stop)
			continue
		}

		step, err := autoGrow.GetStep(*size)
		if err != nil {
			return autoGrowPlan{}, fmt.Errorf("invalid autoGrow step of the %s volume: %w", volume.group, err)
		}
		target := size.DeepCopy()
		target.Add(step)
		if target.Cmp(maxSize) > 0 {
			target = maxSize
		}
		plan.sizes[volume.group] = target
		plan.steps = append(plan.steps, autoGrowStep{
			group:       volume.group,
			from:        *size,
			to:          target,
			instance:    instance,
			usedPercent: usedPercent,
		})
	}

	return plan, nil
}

// reportAutoGrowSteps records a Normal event for each volume grown, unless
// the API server refused the expansion of one of its PVCs: the refusal is
// reported by the VolumeExpansionRefused condition, and the same step is
// attempted again at the next reconciliation
func reportAutoGrowSteps(
	ctx context.Context,
	recorder record.EventRecorder,
	cluster *apiv1.Cluster,
	pvcs []corev1.PersistentVolumeClaim,
	steps []autoGrowStep,
	refusals []*volumeExpansionRefusal,
) {
	refusedGroups := make(map[pvcGroup]bool, len(refusals))
	for _, refusal := range refusals {
		for idx := range pvcs {
			if pvcs[idx].Name == refusal.pvcName {
				refusedGroups[pvcGroupFromLabels(pvcs[idx].Labels)] = true
			}
		}
	}

	contextLogger := log.FromContext(ctx)
	for _, step := range steps {
		if refusedGroups[step.group] {
			continue
		}
		message := fmt.Sprintf("Growing the PVCs of the %s volume from %s to %s: %s uses %.0f%% of it",
			step.group, step.from.String(), step.to.String(), step.instance, step.usedPercent)
		contextLogger.Info(message)
		recorder.Event(cluster, corev1.EventTypeNormal, "VolumeAutoGrow", message)
	}
}

// autoGrowStoppedCondition builds the condition reporting the volumes above
// their threshold that are not grown, sorted by volume
func autoGrowStoppedCondition(stops []autoGrowStop) metav1.Condition {
	slices.SortFunc(stops, func(a, b autoGrowStop) int {
		return strings.Compare(a.group.String(), b.group.String())
	})

	messages := make([]string, 0, len(stops))
	for _, stop := range stops {
		var why string
		switch stop.reason {
		case apiv1.ConditionReasonWALArchivingFailing:
			why = "the WAL archiving is failing, growing the volume would hide it; fix the archiving"
		default:
			why = "it reached the autoGrow maxSize; raise maxSize, or the size, to grow it again"
		}
		messages = append(messages, fmt.Sprintf("the %s volume (%s) is above its threshold of %d%% and is not grown: %s",
			stop.group, stop.size.String(), stop.threshold, why))
	}

	return metav1.Condition{
		Type:    string(apiv1.ConditionVolumeAutoGrowStopped),
		Status:  metav1.ConditionTrue,
		Reason:  string(stops[0].reason),
		Message: strings.ToUpper(messages[0][:1]) + strings.Join(messages, "; ")[1:],
	}
}

// reportAutoGrowStops sets the VolumeAutoGrowStopped condition from the
// stops of this reconciliation, and records an Event when it changes. A
// cluster that never had a stop gets no condition.
func reportAutoGrowStops(
	ctx context.Context,
	c client.Client,
	recorder record.EventRecorder,
	cluster *apiv1.Cluster,
	stops []autoGrowStop,
) error {
	existing := meta.FindStatusCondition(cluster.Status.Conditions, string(apiv1.ConditionVolumeAutoGrowStopped))

	var condition metav1.Condition
	switch {
	case len(stops) > 0:
		condition = autoGrowStoppedCondition(stops)
	case existing == nil || existing.Status == metav1.ConditionFalse:
		return nil
	default:
		condition = metav1.Condition{
			Type:    string(apiv1.ConditionVolumeAutoGrowStopped),
			Status:  metav1.ConditionFalse,
			Reason:  string(apiv1.ConditionReasonVolumeAutoGrowResumed),
			Message: "No volume with autoGrow is stopped above its threshold",
		}
	}
	return setReportedCondition(ctx, c, recorder, cluster, existing, condition)
}
