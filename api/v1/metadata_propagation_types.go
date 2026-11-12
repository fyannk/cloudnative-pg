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

// MetadataPropagationTarget is a kind of object a metadata propagation rule
// applies to
// +kubebuilder:validation:Enum=all;allPods;pgPods;majorUpgradePods;poolerPods;poolerDeployments;services;persistentVolumeClaims;secrets;podDisruptionBudgets;backups;volumeSnapshots
type MetadataPropagationTarget string

// MetadataPropagationRule puts labels and annotations on the objects of the
// targeted kinds. When several rules give the same key to an object, the
// most specific target wins: a kind of Pod over allPods, any kind over all
type MetadataPropagationRule struct {
	// The kinds of objects the rule applies to
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=12
	// +listType=set
	Targets []MetadataPropagationTarget `json:"targets"`

	// Labels to put on the objects. The labels the operator sets itself are
	// never taken from here
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations to put on the objects. The annotations the operator sets
	// itself are never taken from here
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}
