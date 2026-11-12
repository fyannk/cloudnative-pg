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
	"errors"
	"fmt"

	"sigs.k8s.io/yaml"

	"github.com/cloudnative-pg/cloudnative-pg/pkg/metadatarules"
)

// ParseDefaultAndMandatoryMetadata compiles the DEFAULT_METADATA and
// MANDATORY_METADATA settings. It is called once the configuration is
// loaded, and the operator does not start on an error: invalid YAML, an
// unknown field, a resource, target, key or label value that is not valid,
// a key given two values for one target of a resource, or a key the
// operator sets itself
func (config *Data) ParseDefaultAndMandatoryMetadata() error {
	defaults, defaultsErr := compileMetadataSetting("DEFAULT_METADATA", config.DefaultMetadata)
	mandatory, mandatoryErr := compileMetadataSetting("MANDATORY_METADATA", config.MandatoryMetadata)
	if err := errors.Join(defaultsErr, mandatoryErr); err != nil {
		return err
	}

	config.defaultMetadata = defaults
	config.mandatoryMetadata = mandatory
	return nil
}

func compileMetadataSetting(setting, value string) (metadatarules.Sets, error) {
	var rules []metadatarules.OperatorRule
	if err := yaml.UnmarshalStrict([]byte(value), &rules); err != nil {
		return nil, fmt.Errorf("%s: not a YAML list of rules: %w", setting, err)
	}

	sets, problems := metadatarules.CompileOperatorRules(rules)
	if len(problems) == 0 {
		return sets, nil
	}

	errs := make([]error, 0, len(problems))
	for _, problem := range problems {
		errs = append(errs, fmt.Errorf("%s%s", setting, problem))
	}
	return nil, errors.Join(errs...)
}

// MetadataLayers returns the layers of labels and annotations of the
// objects of a resource: the default and mandatory rules of the operator
// for its kind around the rules of the resource
func (config *Data) MetadataLayers(resource metadatarules.Resource, own metadatarules.Set) metadatarules.Layers {
	return metadatarules.Layers{
		Defaults:  config.defaultMetadata.For(resource),
		Own:       own,
		Mandatory: config.mandatoryMetadata.For(resource),
	}
}
