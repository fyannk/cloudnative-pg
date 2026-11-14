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

package metadatarules

import (
	"cmp"
	"fmt"
	"slices"
)

// Resource is a kind of resource the operator creates objects for
type Resource string

const (
	// ClusterResource is the Cluster
	ClusterResource Resource = "Cluster"

	// PoolerResource is the Pooler
	PoolerResource Resource = "Pooler"

	// ScheduledBackupResource is the ScheduledBackup
	ScheduledBackupResource Resource = "ScheduledBackup"

	// BackupResource is the Backup
	BackupResource Resource = "Backup"
)

// Resources lists every kind of resource
var Resources = []Resource{ClusterResource, PoolerResource, ScheduledBackupResource, BackupResource}

// Targets returns the targets of the objects a resource creates, those its
// own rules can name
func (r Resource) Targets() []Target {
	switch r {
	case ClusterResource:
		return []Target{
			All, AllPods, PgPods, MajorUpgradePods, Services, PersistentVolumeClaims, Secrets,
			PodDisruptionBudgets,
		}
	case PoolerResource:
		return []Target{All, PoolerPods, PoolerDeployments, Services, Secrets}
	case ScheduledBackupResource:
		return []Target{Backups}
	case BackupResource:
		return []Target{VolumeSnapshots}
	}
	return nil
}

// Covers tells whether a target reaches objects of a resource: one of its
// targets, or a target covering one of them
func (r Resource) Covers(target Target) bool {
	for _, own := range r.Targets() {
		for covered, found := own, true; found; covered, found = covered.Parent() {
			if covered == target {
				return true
			}
		}
	}
	return false
}

// coveringTargets lists the targets reaching objects of a resource
func (r Resource) coveringTargets() []Target {
	var result []Target
	for _, target := range Targets {
		if r.Covers(target) {
			result = append(result, target)
		}
	}
	return result
}

// OperatorRule is a rule of the operator configuration: it applies to the
// objects of the resources it names, or of every resource when it names
// none
type OperatorRule struct {
	// Resources are the kinds of resources whose objects the rule applies to
	Resources []Resource `json:"resources,omitempty"`

	Rule
}

// Sets are the compiled operator rules: a Set per kind of resource. The zero
// value gives nothing
type Sets map[Resource]Set

// For returns the Set of a kind of resource
func (s Sets) For(resource Resource) Set {
	return s[resource]
}

// CompileOperatorRules checks a list of operator rules and resolves it per
// kind of resource. On top of the problems Compile reports, a rule is
// invalid with an unknown resource, or a target not reaching the objects of
// a resource it names. A rule naming no resource applies to each resource
// through the targets reaching its objects. Two rules only conflict on the
// resources they both apply to
func CompileOperatorRules(rules []OperatorRule) (Sets, []Problem) {
	var problems []Problem
	plain := make([]Rule, len(rules))
	for i, rule := range rules {
		plain[i] = rule.Rule
		for j, resource := range rule.Resources {
			if !slices.Contains(Resources, resource) {
				problems = append(problems, Problem{
					Rule: i, Field: "resources", Target: j, Severity: Invalid,
					Message: fmt.Sprintf("unknown resource %q, expected one of %v", resource, Resources),
				})
			}
		}
	}

	sets := make(Sets, len(Resources))
	reported := make(map[Problem]bool)
	for _, problem := range problems {
		reported[problem] = true
	}
	for _, resource := range Resources {
		set, resourceProblems := compile(plain, func(i int, target Target) (bool, string) {
			named := rules[i].Resources
			switch {
			case len(named) > 0 && !slices.Contains(named, resource):
				return false, ""
			case resource.Covers(target):
				return true, ""
			case len(named) == 0:
				return false, ""
			}
			return false, fmt.Sprintf("target %q not available on a %s, expected one of %v",
				target, resource, resource.coveringTargets())
		})
		for _, problem := range resourceProblems {
			if !reported[problem] {
				reported[problem] = true
				problems = append(problems, problem)
			}
		}
		sets[resource] = set
	}

	slices.SortStableFunc(problems, func(a, b Problem) int {
		return cmp.Compare(a.Rule, b.Rule)
	})
	return sets, problems
}
