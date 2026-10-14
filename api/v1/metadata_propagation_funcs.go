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
	"github.com/cloudnative-pg/cloudnative-pg/pkg/metadatarules"
)

// ToMetadataRules converts the metadata propagation rules of a resource to
// the rules the operator compiles
func ToMetadataRules(rules []MetadataPropagationRule) []metadatarules.Rule {
	if len(rules) == 0 {
		return nil
	}

	result := make([]metadatarules.Rule, len(rules))
	for i, rule := range rules {
		targets := make([]metadatarules.Target, len(rule.Targets))
		for j, target := range rule.Targets {
			targets[j] = metadatarules.Target(target)
		}
		result[i] = metadatarules.Rule{Targets: targets, Labels: rule.Labels, Annotations: rule.Annotations}
	}
	return result
}
