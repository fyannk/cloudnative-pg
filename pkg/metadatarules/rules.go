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

// Package metadatarules holds the rules putting labels and annotations on
// the objects the operator creates: the rules of the resources and the
// default and mandatory rules of the operator configuration share them.
package metadatarules

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"
)

// Target is a kind of object a rule applies to
type Target string

const (
	// All is every object the operator creates
	All Target = "all"

	// AllPods is every Pod the operator creates: PostgreSQL instance,
	// major upgrade and Pooler Pods
	AllPods Target = "allPods"

	// PgPods is the PostgreSQL instance Pods
	PgPods Target = "pgPods"

	// MajorUpgradePods is the Pods of the major upgrade Jobs
	MajorUpgradePods Target = "majorUpgradePods"

	// PoolerPods is the Pods of the Poolers
	PoolerPods Target = "poolerPods"

	// PoolerDeployments is the Deployments of the Poolers
	PoolerDeployments Target = "poolerDeployments"

	// Services is the Services of the Clusters and Poolers
	Services Target = "services"

	// PersistentVolumeClaims is the data, WAL and tablespace PVCs
	PersistentVolumeClaims Target = "persistentVolumeClaims"

	// Secrets is the Secrets the operator creates: credentials, CA and
	// certificates, pull Secrets
	Secrets Target = "secrets"

	// PodDisruptionBudgets is the PodDisruptionBudgets of the Clusters
	PodDisruptionBudgets Target = "podDisruptionBudgets"

	// Backups is the Backups created by the ScheduledBackups
	Backups Target = "backups"

	// VolumeSnapshots is the VolumeSnapshots of the Backups
	VolumeSnapshots Target = "volumeSnapshots"
)

// Targets lists every target
var Targets = []Target{
	All, AllPods, PgPods, MajorUpgradePods, PoolerPods, PoolerDeployments, Services,
	PersistentVolumeClaims, Secrets, PodDisruptionBudgets, Backups, VolumeSnapshots,
}

// IsPod tells whether a target is a kind of Pod, which AllPods covers
func (t Target) IsPod() bool {
	switch t {
	case PgPods, MajorUpgradePods, PoolerPods:
		return true
	}
	return false
}

// Parent returns the target covering a target: AllPods for the kinds of
// Pods, All for the others. All has none
func (t Target) Parent() (Target, bool) {
	switch {
	case t == All:
		return "", false
	case t.IsPod():
		return AllPods, true
	default:
		return All, true
	}
}

// Rule puts labels and annotations on the objects of the targeted kinds
type Rule struct {
	// Targets are the kinds of objects the rule applies to
	Targets []Target `json:"targets"`

	// Labels to put on the objects
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations to put on the objects
	Annotations map[string]string `json:"annotations,omitempty"`
}

// Severity tells what a problem of a rule implies
type Severity string

const (
	// Invalid problems make the rules unusable
	Invalid Severity = "invalid"

	// Ignored problems are keys the operator sets itself, never applied
	Ignored Severity = "ignored"
)

// Problem is something wrong in a list of rules
type Problem struct {
	// Rule is the index of the rule
	Rule int

	// Field is "targets", "labels" or "annotations"
	Field string

	// Target is the index of the target in the targets of the rule, or of
	// the resource in its resources, or -1
	Target int

	// Key is the label or annotation key, if any
	Key string

	// Severity of the problem
	Severity Severity

	// Message describes the problem
	Message string
}

// Path locates the problem, as `[0].targets[2]` or `[1].labels["team"]`
func (p Problem) Path() string {
	path := fmt.Sprintf("[%d].%s", p.Rule, p.Field)
	switch {
	case p.Target >= 0:
		path += fmt.Sprintf("[%d]", p.Target)
	case p.Field != "targets":
		path += fmt.Sprintf("[%q]", p.Key)
	}
	return path
}

func (p Problem) String() string {
	return fmt.Sprintf("%s: %s", p.Path(), p.Message)
}

// Set is a compiled list of rules: per target, the labels and annotations
// it gives. The zero value is an empty Set
type Set struct {
	labels      map[Target]map[string]string
	annotations map[Target]map[string]string
}

// Compile checks a list of rules and resolves it. The allowed targets are
// those of the resource the rules come from. A rule is invalid with an
// unknown or not allowed target, no target, an invalid key or label value,
// or a key given two values for one target by two rules. A key the operator
// sets itself is ignored. The Set holds every valid key, so that the valid
// part of the rules still applies
func Compile(rules []Rule, allowed []Target) (Set, []Problem) {
	return compile(rules, func(_ int, target Target) (bool, string) {
		if slices.Contains(allowed, target) {
			return true, ""
		}
		return false, fmt.Sprintf("target %q not available here, expected one of %v", target, allowed)
	})
}

// compile checks and resolves a list of rules. For each known target of a
// rule, accept tells whether the rule applies to it, and why not when that
// is a problem: a target skipped without a reason is left out silently
func compile(rules []Rule, accept func(rule int, target Target) (bool, string)) (Set, []Problem) {
	var problems []Problem
	set := Set{labels: map[Target]map[string]string{}, annotations: map[Target]map[string]string{}}
	labelSources := map[Target]map[string]int{}
	annotationSources := map[Target]map[string]int{}

	for i, rule := range rules {
		if len(rule.Targets) == 0 {
			problems = append(problems, Problem{
				Rule: i, Field: "targets", Target: -1, Severity: Invalid,
				Message: "no target",
			})
		}

		var targets []Target
		for j, target := range rule.Targets {
			if !slices.Contains(Targets, target) {
				problems = append(problems, Problem{
					Rule: i, Field: "targets", Target: j, Severity: Invalid,
					Message: fmt.Sprintf("unknown target %q, expected one of %v", target, Targets),
				})
				continue
			}
			accepted, refusal := accept(i, target)
			switch {
			case accepted:
				targets = append(targets, target)
			case refusal != "":
				problems = append(problems, Problem{
					Rule: i, Field: "targets", Target: j, Severity: Invalid, Message: refusal,
				})
			}
		}

		problems = append(problems, compileKind("labels", rule.Labels, true, targets,
			set.labels, labelSources, i)...)
		problems = append(problems, compileKind("annotations", rule.Annotations, false, targets,
			set.annotations, annotationSources, i)...)
	}

	return set, problems
}

func compileKind(
	field string,
	values map[string]string,
	labels bool,
	targets []Target,
	into map[Target]map[string]string,
	sources map[Target]map[string]int,
	rule int,
) []Problem {
	var problems []Problem
	problem := func(key string, severity Severity, message string) Problem {
		return Problem{Rule: rule, Field: field, Target: -1, Key: key, Severity: severity, Message: message}
	}
	for _, key := range slices.Sorted(maps.Keys(values)) {
		value := values[key]
		if errs := validation.IsQualifiedName(key); len(errs) > 0 {
			problems = append(problems, problem(key, Invalid, "invalid key: "+strings.Join(errs, ", ")))
			continue
		}
		if labels {
			if errs := validation.IsValidLabelValue(value); len(errs) > 0 {
				problems = append(problems, problem(key, Invalid, "invalid label value: "+strings.Join(errs, ", ")))
				continue
			}
		}
		if utils.IsOperatorMetadataKey(key) {
			problems = append(problems, problem(key, Ignored, "set by the operator, never applied"))
			continue
		}

		for _, target := range targets {
			if into[target] == nil {
				into[target] = map[string]string{}
				sources[target] = map[string]int{}
			}
			if previous, found := into[target][key]; found && previous != value {
				problems = append(problems, problem(key, Invalid, fmt.Sprintf(
					"%q on %s: also set to %q by rule [%d]", value, target, previous, sources[target][key])))
				continue
			}
			into[target][key] = value
			sources[target][key] = rule
		}
	}
	return problems
}

// IsEmpty tells whether the Set gives no label and no annotation
func (s Set) IsEmpty() bool {
	return len(s.labels) == 0 && len(s.annotations) == 0
}

// Labels returns the labels the Set gives to an object of a kind: those of
// the targets covering it, the most specific winning. Nil when none
func (s Set) Labels(target Target) map[string]string {
	return lookup(s.labels, target)
}

// Annotations returns the annotations the Set gives to an object of a kind,
// as Labels does
func (s Set) Annotations(target Target) map[string]string {
	return lookup(s.annotations, target)
}

// Reaches tells whether the Set gives a label or an annotation to an object
// of a kind
func (s Set) Reaches(target Target) bool {
	return len(s.Labels(target)) > 0 || len(s.Annotations(target)) > 0
}

func lookup(values map[Target]map[string]string, target Target) map[string]string {
	var result map[string]string
	if parent, found := target.Parent(); found {
		result = lookup(values, parent)
	}
	if len(values[target]) > 0 {
		if result == nil {
			result = make(map[string]string, len(values[target]))
		}
		maps.Copy(result, values[target])
	}
	return result
}

// Layers are the sources of the labels and annotations of an object, from
// the lowest to the highest
type Layers struct {
	// Defaults are the default rules of the operator: applied when no
	// higher layer sets the key
	Defaults Set

	// Own are the rules of the resource
	Own Set

	// Mandatory are the mandatory rules of the operator: they win over
	// every other layer
	Mandatory Set
}

// Labels stacks the labels of an object of a kind: the defaults, the
// labels the resource already gives (inherited metadata, templates), its
// rules, the mandatory values. The result is a new map
func (l Layers) Labels(target Target, resource map[string]string) map[string]string {
	return stack(l.Defaults.Labels(target), resource, l.Own.Labels(target), l.Mandatory.Labels(target))
}

// Annotations stacks the annotations of an object of a kind, as Labels does
func (l Layers) Annotations(target Target, resource map[string]string) map[string]string {
	return stack(l.Defaults.Annotations(target), resource, l.Own.Annotations(target),
		l.Mandatory.Annotations(target))
}

// Reaches tells whether a rule of a layer gives a label or an annotation to
// an object of a kind
func (l Layers) Reaches(target Target) bool {
	return l.Defaults.Reaches(target) || l.Own.Reaches(target) || l.Mandatory.Reaches(target)
}

func stack(layers ...map[string]string) map[string]string {
	size := 0
	for _, layer := range layers {
		size += len(layer)
	}
	result := make(map[string]string, size)
	for _, layer := range layers {
		maps.Copy(result, layer)
	}
	return result
}
