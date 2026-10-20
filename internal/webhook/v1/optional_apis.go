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

package v1

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/internal/configuration"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"
)

// An object asking for an optional API the operator does not use is
// refused when it cannot work without it, on creation or when the field
// changes. When the field is unchanged, the object was admitted before the
// API was disabled, and the update is only warned about: the operator and
// the users must still be able to update it. When the object works without
// the API, it is only warned about.

// isAdmissionRequest tells a real admission request, which carries the
// request in its context, from the admission guard of the reconcilers, which
// validates stored objects through ValidateCreate: a stored object was
// admitted before, and is treated as unchanged
func isAdmissionRequest(ctx context.Context) bool {
	_, err := admission.RequestFromContext(ctx)
	return err == nil
}

// unavailableAPIMessage says that an optional API is not used by this operator
func unavailableAPIMessage(api, setting string) string {
	return fmt.Sprintf("the %s API is not available to this operator "+
		"(not installed, forbidden, or disabled by %s)", api, setting)
}

var (
	clusterImageCatalogUnavailable = unavailableAPIMessage("ClusterImageCatalog", "API_CLUSTER_IMAGE_CATALOG_ENABLED") +
		", use a namespaced ImageCatalog"
	volumeSnapshotUnavailable = unavailableAPIMessage("VolumeSnapshot", "API_VOLUME_SNAPSHOT_ENABLED") +
		"; if the VolumeSnapshot CRD was installed after the operator started, restart the operator"
	podMonitorUnavailable = unavailableAPIMessage("PodMonitor", "API_POD_MONITOR_ENABLED") +
		": no PodMonitor is created, create one by hand"
)

// unavailableAPIProblems collects the errors and the warnings about the
// fields asking for an unavailable optional API
type unavailableAPIProblems struct {
	errs     field.ErrorList
	warnings admission.Warnings
}

// required reports a field without which the object cannot work: an error
// when the field is new or changed, a warning otherwise
func (p *unavailableAPIProblems) required(path *field.Path, changed bool, message string) {
	if changed {
		p.errs = append(p.errs, field.Forbidden(path, message))
		return
	}
	p.warnings = append(p.warnings, fmt.Sprintf("%s: %s", path, message))
}

// optional reports a field the object works without: always a warning
func (p *unavailableAPIProblems) optional(path *field.Path, message string) {
	p.warnings = append(p.warnings, fmt.Sprintf("%s: %s", path, message))
}

// getClusterUnavailableAPIProblems checks the fields of a Cluster asking for
// an unavailable optional API. old is nil on creation
func getClusterUnavailableAPIProblems(r, old *apiv1.Cluster) (field.ErrorList, admission.Warnings) {
	var problems unavailableAPIProblems

	if ref := r.Spec.ImageCatalogRef; ref != nil && ref.Kind == apiv1.ClusterImageCatalogKind &&
		!configuration.Current.APIClusterImageCatalogEnabled {
		changed := old == nil || old.Spec.ImageCatalogRef == nil ||
			old.Spec.ImageCatalogRef.Kind != ref.Kind || old.Spec.ImageCatalogRef.Name != ref.Name
		problems.required(field.NewPath("spec", "imageCatalogRef"), changed, clusterImageCatalogUnavailable)
	}

	if hasSnapshotRecovery(r) && !utils.HaveVolumeSnapshot() {
		problems.required(field.NewPath("spec", "bootstrap", "recovery", "volumeSnapshots"),
			old == nil || !hasSnapshotRecovery(old), volumeSnapshotUnavailable)
	}

	if r.Spec.Backup != nil && r.Spec.Backup.VolumeSnapshot != nil && !utils.HaveVolumeSnapshot() {
		problems.optional(field.NewPath("spec", "backup", "volumeSnapshot"),
			volumeSnapshotUnavailable+": no snapshot backup can be taken, and replicas are created with pg_basebackup")
	}

	if r.IsPodMonitorEnabled() && !configuration.Current.APIPodMonitorEnabled {
		problems.optional(field.NewPath("spec", "monitoring", "enablePodMonitor"), podMonitorUnavailable)
	}

	if configuration.Current.NodeStatusSource == configuration.NodeStatusSourceNone {
		if old == nil {
			problems.warnings = append(problems.warnings, configuration.NodeStatusUnavailableMessage)
		}
		if _, nodeLabelKeys := r.GetTopologyLabelKeys(); len(nodeLabelKeys) > 0 {
			path := field.NewPath("spec", "postgresql", "syncReplicaElectionConstraint", "nodeLabelsAntiAffinity")
			if sync := r.Spec.PostgresConfiguration.Synchronous; sync != nil && len(sync.NodeFailureDomainKeys) > 0 {
				path = field.NewPath("spec", "postgresql", "synchronous", "nodeFailureDomainKeys")
			}
			problems.optional(path, "the operator reads no node state (NODE_STATUS_SOURCE=none): the node labels "+
				strings.Join(nodeLabelKeys, ", ")+" cannot be resolved and the failure domain constraints "+
				"of the synchronous replicas are not applied")
		}
	}

	return problems.errs, problems.warnings
}

func hasSnapshotRecovery(r *apiv1.Cluster) bool {
	return r.Spec.Bootstrap != nil && r.Spec.Bootstrap.Recovery != nil &&
		r.Spec.Bootstrap.Recovery.VolumeSnapshots != nil
}

// getPoolerUnavailableAPIProblems checks the fields of a Pooler asking for
// an unavailable optional API. old is nil on creation
func getPoolerUnavailableAPIProblems(r, old *apiv1.Pooler) (field.ErrorList, admission.Warnings) {
	var problems unavailableAPIProblems

	if ref := poolerImageCatalogRef(r); ref != nil && ref.Kind == apiv1.ClusterImageCatalogKind &&
		!configuration.Current.APIClusterImageCatalogEnabled {
		var oldRef *apiv1.ImageCatalogComponentRef
		if old != nil {
			oldRef = poolerImageCatalogRef(old)
		}
		changed := oldRef == nil || oldRef.Kind != ref.Kind || oldRef.Name != ref.Name
		problems.required(field.NewPath("spec", "pgbouncer", "imageCatalogRef"), changed,
			clusterImageCatalogUnavailable)
	}

	//nolint:staticcheck // the deprecated field is what asks for a PodMonitor
	if r.Spec.Monitoring != nil && r.Spec.Monitoring.EnablePodMonitor &&
		!configuration.Current.APIPodMonitorEnabled {
		problems.optional(field.NewPath("spec", "monitoring", "enablePodMonitor"), podMonitorUnavailable)
	}

	return problems.errs, problems.warnings
}

func poolerImageCatalogRef(r *apiv1.Pooler) *apiv1.ImageCatalogComponentRef {
	if r.Spec.PgBouncer == nil {
		return nil
	}
	return r.Spec.PgBouncer.ImageCatalogRef
}

// volumeSnapshotMethodErrors refuses the volumeSnapshot method of a Backup or
// a ScheduledBackup when the VolumeSnapshot API is not available
func volumeSnapshotMethodErrors(method apiv1.BackupMethod) field.ErrorList {
	if method != apiv1.BackupMethodVolumeSnapshot || utils.HaveVolumeSnapshot() {
		return nil
	}
	return field.ErrorList{field.Invalid(field.NewPath("spec", "method"), method, volumeSnapshotUnavailable)}
}

// volumeSnapshotMethodOnUpdate turns the refusal of an unchanged
// volumeSnapshot method into a warning: the object was admitted before the
// API became unavailable, and must stay updatable
func volumeSnapshotMethodOnUpdate(
	errs field.ErrorList,
	method, oldMethod apiv1.BackupMethod,
) (field.ErrorList, admission.Warnings) {
	if method != oldMethod || len(volumeSnapshotMethodErrors(method)) == 0 {
		return errs, nil
	}

	var kept field.ErrorList
	for _, err := range errs {
		if err.Field == "spec.method" && err.Detail == volumeSnapshotUnavailable {
			continue
		}
		kept = append(kept, err)
	}
	return kept, admission.Warnings{fmt.Sprintf("spec.method: %s", volumeSnapshotUnavailable)}
}
