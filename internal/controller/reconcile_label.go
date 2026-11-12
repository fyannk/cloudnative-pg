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

package controller

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"
)

const reconcileLabelValue = "true"

// hasReconcileLabel tells whether a set of labels carries the label the
// operator sets on every object it manages
func hasReconcileLabel(labels map[string]string) bool {
	return labels[utils.ReconcileLabelName] == reconcileLabelValue
}

// isReconcileLabelled tells whether an object carries the label the operator
// sets on every object it manages
func isReconcileLabelled(obj client.Object) bool {
	return hasReconcileLabel(obj.GetLabels())
}

// ensureReconcileLabel adds the label the operator sets on every object it
// manages to an existing object created before the label was introduced
func ensureReconcileLabel(ctx context.Context, cli client.Client, obj client.Object) error {
	if isReconcileLabelled(obj) {
		return nil
	}

	orig := obj.DeepCopyObject().(client.Object)
	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[utils.ReconcileLabelName] = reconcileLabelValue
	obj.SetLabels(labels)
	return cli.Patch(ctx, obj, client.MergeFrom(orig))
}
