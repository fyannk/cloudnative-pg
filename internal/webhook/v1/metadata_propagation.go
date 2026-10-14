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
	"fmt"
	"maps"
	"slices"

	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/internal/configuration"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/metadatarules"
)

// validateMetadataPropagation checks the metadataPropagation rules of a
// resource: an invalid rule is refused, a key the operator sets itself is
// warned about, as it is never applied
func validateMetadataPropagation(
	rules []apiv1.MetadataPropagationRule,
	allowed []metadatarules.Target,
) (field.ErrorList, admission.Warnings) {
	_, problems := metadatarules.Compile(apiv1.ToMetadataRules(rules), allowed)

	var errs field.ErrorList
	var warnings admission.Warnings
	for _, problem := range problems {
		path := field.NewPath("spec", "metadataPropagation").Index(problem.Rule).Child(problem.Field)
		var value any
		switch {
		case problem.Target >= 0:
			path = path.Index(problem.Target)
			value = rules[problem.Rule].Targets[problem.Target]
		case problem.Field == "targets":
			value = rules[problem.Rule].Targets
		default:
			path = path.Key(problem.Key)
			value = problem.Key
		}

		if problem.Severity == metadatarules.Ignored {
			warnings = append(warnings, fmt.Sprintf("%s: %s", path, problem.Message))
			continue
		}
		errs = append(errs, field.Invalid(path, value, problem.Message))
	}

	return errs, warnings
}

// getReplacedByMandatoryWarnings warns about the labels and annotations a
// resource sets on its objects and MANDATORY_METADATA replaces. The kinds
// are visited in order, a kind after the one covering it: a replacement
// already reported for the covering kind is not reported again
func getReplacedByMandatoryWarnings(
	resource metadatarules.Resource,
	targets []metadatarules.Target,
	labelsFor, annotationsFor func(metadatarules.Target) map[string]string,
) admission.Warnings {
	mandatory := configuration.Current.MetadataLayers(resource, metadatarules.Set{}).Mandatory
	if mandatory.IsEmpty() {
		return nil
	}

	var result admission.Warnings
	for _, target := range targets {
		parent, hasParent := target.Parent()
		for _, kind := range []struct {
			name      string
			valuesFor func(metadatarules.Target) map[string]string
			mandatory func(metadatarules.Target) map[string]string
		}{
			{"label", labelsFor, mandatory.Labels},
			{"annotation", annotationsFor, mandatory.Annotations},
		} {
			values, imposed := kind.valuesFor(target), kind.mandatory(target)
			var parentValues, parentImposed map[string]string
			if hasParent {
				parentValues, parentImposed = kind.valuesFor(parent), kind.mandatory(parent)
			}
			for _, key := range slices.Sorted(maps.Keys(imposed)) {
				value, found := values[key]
				if !found || value == imposed[key] {
					continue
				}
				if parentValue, parentFound := parentValues[key]; parentFound && parentValue == value &&
					parentImposed[key] == imposed[key] {
					continue
				}
				result = append(result, fmt.Sprintf(
					"%s %q: %q set on this %s is replaced with %q by MANDATORY_METADATA on %s",
					kind.name, key, value, resource, imposed[key], target))
			}
		}
	}

	return result
}
