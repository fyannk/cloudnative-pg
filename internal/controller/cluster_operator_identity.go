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
	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/internal/configuration"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/versions"
)

// operatorStatus builds what the operator says about itself in the status of
// a reconciled Cluster, as far as the configuration lets it: nil when nothing
// is to be written, otherwise the version, the Deployment name when reported
// and known, and the namespace when reported
func operatorStatus(config *configuration.Data) *apiv1.OperatorStatus {
	if !config.ReportsOperatorVersion() {
		return nil
	}
	status := &apiv1.OperatorStatus{Version: versions.Version}
	if config.ReportsOperatorName() {
		status.Name = config.OperatorDeploymentName
	}
	if config.ReportsOperatorNamespace() {
		status.Namespace = config.OperatorNamespace
	}
	return status
}
