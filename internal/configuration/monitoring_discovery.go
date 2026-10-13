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

package configuration

import (
	"fmt"
	"slices"

	"github.com/cloudnative-pg/cloudnative-pg/pkg/monitoringdiscovery"
)

// ParseMonitoringDiscoveryAnnotations parses the
// MONITORING_DISCOVERY_ANNOTATIONS setting. It is called once the
// configuration is loaded, and the operator does not start on an error
func (config *Data) ParseMonitoringDiscoveryAnnotations() error {
	families, err := monitoringdiscovery.ParseFamilies(config.MonitoringDiscoveryAnnotations)
	if err != nil {
		return fmt.Errorf("MONITORING_DISCOVERY_ANNOTATIONS: %w", err)
	}

	config.monitoringDiscoveryFamilies = families
	return nil
}

// MonitoringDiscoveryFamilies returns the families of discovery annotations
// set on the instance and Pooler Pods, none by default
func (config *Data) MonitoringDiscoveryFamilies() []monitoringdiscovery.Family {
	return slices.Clone(config.monitoringDiscoveryFamilies)
}
