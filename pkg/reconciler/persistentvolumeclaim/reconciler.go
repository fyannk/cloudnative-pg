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
	"time"

	"github.com/cloudnative-pg/machinery/pkg/log"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/postgres"
)

// Reconcile reconciles the PVCs
func Reconcile(
	ctx context.Context,
	c client.Client,
	recorder record.EventRecorder,
	cluster *apiv1.Cluster,
	instances []corev1.Pod,
	pvcs []corev1.PersistentVolumeClaim,
	instancesStatus postgres.PostgresqlStatusList,
) (ctrl.Result, error) {
	if res, err := reconcileMultipleInstancesMissingPVCs(ctx, c, cluster, instances, pvcs); !res.IsZero() || err != nil {
		return res, err
	}

	return ReconcileExisting(ctx, c, recorder, cluster, pvcs, instancesStatus)
}

// ReconcileExisting resizes the existing PVCs, growing the volumes with
// autoGrow, and reports the refused expansions and the stopped autoGrow
func ReconcileExisting(
	ctx context.Context,
	c client.Client,
	recorder record.EventRecorder,
	cluster *apiv1.Cluster,
	pvcs []corev1.PersistentVolumeClaim,
	instancesStatus postgres.PostgresqlStatusList,
) (ctrl.Result, error) {
	contextLogger := log.FromContext(ctx)

	plan, err := planAutoGrow(cluster, pvcs, instancesStatus)
	if err != nil {
		return ctrl.Result{}, err
	}

	refusals, err := reconcileExistingPVCs(ctx, c, cluster, pvcs, plan.sizes)
	if err != nil {
		if apierrs.IsConflict(err) {
			contextLogger.Debug("Conflict error while reconciling PVCs", "error", err)
			return ctrl.Result{RequeueAfter: time.Second}, nil
		}

		return ctrl.Result{}, err
	}

	// a refused expansion is reported and does not stop the reconciliation:
	// the cluster requeues itself to retry it, see VolumeExpansionRetryInterval
	if err := reportVolumeExpansion(ctx, c, recorder, cluster, refusals); err != nil {
		return ctrl.Result{}, err
	}

	reportAutoGrowSteps(ctx, recorder, cluster, pvcs, plan.steps, refusals)
	if err := reportAutoGrowStops(ctx, c, recorder, cluster, plan.stops); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
