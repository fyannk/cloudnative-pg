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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("The targets", func() {
	DescribeTable("know the kinds of Pods and the target covering each one",
		func(target Target, isPod bool, parent Target) {
			Expect(target.IsPod()).To(Equal(isPod))
			got, found := target.Parent()
			Expect(found).To(Equal(parent != ""))
			Expect(got).To(Equal(parent))
		},
		Entry("all", All, false, Target("")),
		Entry("allPods", AllPods, false, All),
		Entry("pgPods", PgPods, true, AllPods),
		Entry("majorUpgradePods", MajorUpgradePods, true, AllPods),
		Entry("poolerPods", PoolerPods, true, AllPods),
		Entry("poolerDeployments", PoolerDeployments, false, All),
		Entry("services", Services, false, All),
		Entry("persistentVolumeClaims", PersistentVolumeClaims, false, All),
		Entry("secrets", Secrets, false, All),
		Entry("podDisruptionBudgets", PodDisruptionBudgets, false, All),
		Entry("backups", Backups, false, All),
		Entry("volumeSnapshots", VolumeSnapshots, false, All),
	)

	It("are all listed, once", func() {
		Expect(Targets).To(HaveLen(12))
		seen := map[Target]bool{}
		for _, target := range Targets {
			Expect(seen).NotTo(HaveKey(target))
			seen[target] = true
		}
	})
})

// problems keeps the path and the severity of each problem, for comparison
func problems(list []Problem) []string {
	result := make([]string, 0, len(list))
	for _, p := range list {
		result = append(result, string(p.Severity)+" "+p.Path())
	}
	return result
}

var _ = Describe("Compiling rules", func() {
	It("accepts no rule at all", func() {
		set, found := Compile(nil, Targets)
		Expect(found).To(BeEmpty())
		Expect(set.IsEmpty()).To(BeTrue())
		Expect(Set{}.IsEmpty()).To(BeTrue())
		Expect(Set{}.Labels(PgPods)).To(BeNil())
	})

	It("puts the labels and annotations of a rule on each of its targets", func() {
		set, found := Compile([]Rule{{
			Targets:     []Target{PgPods, Services},
			Labels:      map[string]string{"team": "payments"},
			Annotations: map[string]string{"example.com/note": "x"},
		}}, Targets)
		Expect(found).To(BeEmpty())
		for _, target := range []Target{PgPods, Services} {
			Expect(set.Labels(target)).To(Equal(map[string]string{"team": "payments"}), string(target))
			Expect(set.Annotations(target)).To(Equal(map[string]string{"example.com/note": "x"}), string(target))
		}
		Expect(set.Labels(Secrets)).To(BeNil())
		Expect(set.Labels(All)).To(BeNil())
	})

	DescribeTable("accepts any annotation value",
		func(value string) {
			set, found := Compile([]Rule{{
				Targets:     []Target{Secrets},
				Annotations: map[string]string{"example.com/value": value},
			}}, Targets)
			Expect(found).To(BeEmpty())
			Expect(set.Annotations(Secrets)).To(HaveKeyWithValue("example.com/value", value))
		},
		Entry("with commas", "payments-app,payments-batch"),
		Entry("with an @", "dba@example.com"),
		Entry("with =", "a=b"),
		Entry("JSON", `{"checks": [1, 2]}`),
		Entry("on several lines", "config:\n  scrape_configs:\n    - job_name: x\n"),
		Entry("empty", ""),
		Entry("long", strings.Repeat("x", 4096)),
	)

	DescribeTable("refuses an invalid rule, with the path of the problem",
		func(rules []Rule, allowed []Target, expected []string) {
			_, found := Compile(rules, allowed)
			Expect(problems(found)).To(Equal(expected))
		},
		Entry("no target",
			[]Rule{{Labels: map[string]string{"a": "1"}}}, Targets,
			[]string{"invalid [0].targets"}),
		Entry("an unknown target",
			[]Rule{{Targets: []Target{"nodes"}, Labels: map[string]string{"a": "1"}}}, Targets,
			[]string{"invalid [0].targets[0]"}),
		Entry("a target with the wrong case",
			[]Rule{{Targets: []Target{"PgPods"}}}, Targets,
			[]string{"invalid [0].targets[0]"}),
		Entry("a target not available to the resource",
			[]Rule{{Targets: []Target{PgPods, PoolerPods}}}, []Target{All, PgPods},
			[]string{"invalid [0].targets[1]"}),
		Entry("an invalid key",
			[]Rule{{Targets: []Target{All}, Labels: map[string]string{"my team": "x"}}}, Targets,
			[]string{`invalid [0].labels["my team"]`}),
		Entry("an empty key",
			[]Rule{{Targets: []Target{All}, Annotations: map[string]string{"": "x"}}}, Targets,
			[]string{`invalid [0].annotations[""]`}),
		Entry("an invalid prefix",
			[]Rule{{Targets: []Target{All}, Labels: map[string]string{"Example.COM/x": "1"}}}, Targets,
			[]string{`invalid [0].labels["Example.COM/x"]`}),
		Entry("a label value with spaces",
			[]Rule{{Targets: []Target{All}, Labels: map[string]string{"team": "a b"}}}, Targets,
			[]string{`invalid [0].labels["team"]`}),
		Entry("a label value with a comma, valid for an annotation",
			[]Rule{{
				Targets: []Target{All}, Labels: map[string]string{"team": "a,b"},
				Annotations: map[string]string{"team": "a,b"},
			}}, Targets,
			[]string{`invalid [0].labels["team"]`}),
		Entry("a label value of 64 characters",
			[]Rule{{Targets: []Target{All}, Labels: map[string]string{"team": strings.Repeat("x", 64)}}}, Targets,
			[]string{`invalid [0].labels["team"]`}),
		Entry("a key the operator sets itself, ignored",
			[]Rule{{
				Targets: []Target{PgPods}, Labels: map[string]string{"cnpg.io/podRole": "x"},
				Annotations: map[string]string{"app.kubernetes.io/name": "x"},
			}}, Targets,
			[]string{`ignored [0].labels["cnpg.io/podRole"]`, `ignored [0].annotations["app.kubernetes.io/name"]`}),
		Entry("every problem at once, in a stable order",
			[]Rule{
				{Targets: []Target{"nodes", PgPods}, Labels: map[string]string{"z": "a b", "a": "1"}},
				{Labels: map[string]string{"cnpg.io/cluster": "x"}},
			}, Targets,
			[]string{
				"invalid [0].targets[0]", `invalid [0].labels["z"]`, "invalid [1].targets",
				`ignored [1].labels["cnpg.io/cluster"]`,
			}),
	)

	It("tells an unknown target from one not available to the resource", func() {
		_, found := Compile([]Rule{{Targets: []Target{"nodes", PoolerPods}}}, []Target{All, PgPods})
		Expect(found).To(HaveLen(2))
		Expect(found[0].Message).To(HavePrefix(`unknown target "nodes", expected one of [all allPods`))
		Expect(found[1].Message).To(HavePrefix(`target "poolerPods" not available here`))
	})

	It("locates each problem by rule, field, target index and key", func() {
		_, found := Compile([]Rule{
			{Targets: []Target{PgPods, "nodes"}, Labels: map[string]string{"team": "a b"}},
			{Annotations: map[string]string{"cnpg.io/x": "1"}},
		}, Targets)
		Expect(found).To(HaveLen(4))
		Expect(found[0]).To(MatchFields(IgnoreExtras, Fields{
			"Rule": Equal(0), "Field": Equal("targets"), "Target": Equal(1), "Key": BeEmpty(),
		}))
		Expect(found[1]).To(MatchFields(IgnoreExtras, Fields{
			"Rule": Equal(0), "Field": Equal("labels"), "Target": Equal(-1), "Key": Equal("team"),
		}))
		Expect(found[2]).To(MatchFields(IgnoreExtras, Fields{
			"Rule": Equal(1), "Field": Equal("targets"), "Target": Equal(-1), "Key": BeEmpty(),
		}))
		Expect(found[3]).To(MatchFields(IgnoreExtras, Fields{
			"Rule": Equal(1), "Field": Equal("annotations"), "Key": Equal("cnpg.io/x"), "Severity": Equal(Ignored),
		}))
		Expect(found[2].Path()).To(Equal("[1].targets"))
		Expect(found[3].Path()).To(Equal(`[1].annotations["cnpg.io/x"]`))
	})

	It("names the expected targets in the message", func() {
		_, found := Compile([]Rule{{Targets: []Target{PoolerPods}}}, []Target{All, PgPods})
		Expect(found[0].Message).To(ContainSubstring(`target "poolerPods" not available here`))
		Expect(found[0].Message).To(ContainSubstring("[all pgPods]"))
		Expect(found[0].String()).To(HavePrefix("[0].targets[0]: "))
	})

	It("keeps the valid part of rules with problems", func() {
		set, found := Compile([]Rule{{
			Targets: []Target{"nodes", PgPods},
			Labels:  map[string]string{"team": "payments", "bad": "a b", "cnpg.io/cluster": "x"},
		}}, Targets)
		Expect(found).To(HaveLen(3))
		Expect(set.Labels(PgPods)).To(Equal(map[string]string{"team": "payments"}))
	})

	Context("with two rules on one target", func() {
		It("accepts the same value twice, and keys that differ", func() {
			set, found := Compile([]Rule{
				{Targets: []Target{PgPods}, Labels: map[string]string{"team": "payments", "a": "1"}},
				{Targets: []Target{PgPods, Services}, Labels: map[string]string{"team": "payments", "b": "2"}},
			}, Targets)
			Expect(found).To(BeEmpty())
			Expect(set.Labels(PgPods)).To(Equal(map[string]string{"team": "payments", "a": "1", "b": "2"}))
		})

		It("refuses a key given two values, naming the other rule", func() {
			_, found := Compile([]Rule{
				{Targets: []Target{Secrets}, Annotations: map[string]string{"x": "1"}},
				{Targets: []Target{Services}, Annotations: map[string]string{"x": "2"}},
				{Targets: []Target{Secrets}, Annotations: map[string]string{"x": "3"}},
			}, Targets)
			Expect(problems(found)).To(Equal([]string{`invalid [2].annotations["x"]`}))
			Expect(found[0].Message).To(Equal(`"3" on secrets: also set to "1" by rule [0]`))
		})

		It("keeps labels and annotations apart", func() {
			_, found := Compile([]Rule{
				{Targets: []Target{PgPods}, Labels: map[string]string{"x": "1"}},
				{Targets: []Target{PgPods}, Annotations: map[string]string{"x": "2"}},
			}, Targets)
			Expect(found).To(BeEmpty())
		})

		It("does not take two levels of targets as a conflict", func() {
			set, found := Compile([]Rule{
				{Targets: []Target{All}, Labels: map[string]string{"tier": "other"}},
				{Targets: []Target{AllPods}, Labels: map[string]string{"tier": "pods"}},
				{Targets: []Target{PgPods}, Labels: map[string]string{"tier": "db"}},
			}, Targets)
			Expect(found).To(BeEmpty())
			Expect(set.Labels(PgPods)).To(HaveKeyWithValue("tier", "db"))
		})
	})
})

var _ = Describe("Looking up the metadata of a kind of object", func() {
	var set Set

	BeforeEach(func() {
		var found []Problem
		set, found = Compile([]Rule{
			{Targets: []Target{PgPods}, Labels: map[string]string{"tier": "db"}},
			{Targets: []Target{All}, Labels: map[string]string{"tier": "other", "cost-centre": "dbaas"}},
			{Targets: []Target{AllPods}, Labels: map[string]string{"tier": "pods", "mesh": "on"}},
			{Targets: []Target{Services}, Annotations: map[string]string{"lb": "internal"}},
		}, Targets)
		Expect(found).To(BeEmpty())
	})

	DescribeTable("gives each kind the most specific value, whatever the order of the rules",
		func(target Target, labels, annotations map[string]string) {
			Expect(set.Labels(target)).To(Equal(labels))
			Expect(set.Annotations(target)).To(Equal(annotations))
		},
		Entry("pgPods: its own, then allPods, then all", PgPods,
			map[string]string{"tier": "db", "mesh": "on", "cost-centre": "dbaas"}, map[string]string(nil)),
		Entry("poolerPods: allPods, then all", PoolerPods,
			map[string]string{"tier": "pods", "mesh": "on", "cost-centre": "dbaas"}, map[string]string(nil)),
		Entry("majorUpgradePods: allPods, then all", MajorUpgradePods,
			map[string]string{"tier": "pods", "mesh": "on", "cost-centre": "dbaas"}, map[string]string(nil)),
		Entry("allPods itself", AllPods,
			map[string]string{"tier": "pods", "mesh": "on", "cost-centre": "dbaas"}, map[string]string(nil)),
		Entry("services: all, and its own annotations", Services,
			map[string]string{"tier": "other", "cost-centre": "dbaas"}, map[string]string{"lb": "internal"}),
		Entry("secrets: all only", Secrets,
			map[string]string{"tier": "other", "cost-centre": "dbaas"}, map[string]string(nil)),
		Entry("all itself", All,
			map[string]string{"tier": "other", "cost-centre": "dbaas"}, map[string]string(nil)),
	)

	It("tells which kinds are reached", func() {
		Expect(set.Reaches(Backups)).To(BeTrue())
		empty, _ := Compile([]Rule{{Targets: []Target{PgPods}, Labels: map[string]string{"a": "1"}}}, Targets)
		Expect(empty.Reaches(PgPods)).To(BeTrue())
		Expect(empty.Reaches(PoolerPods)).To(BeFalse())
		Expect(empty.Reaches(AllPods)).To(BeFalse())
		Expect(empty.Reaches(All)).To(BeFalse())
	})

	It("returns copies, which the caller may change", func() {
		labels := set.Labels(PgPods)
		labels["tier"] = "changed"
		delete(labels, "mesh")
		Expect(set.Labels(PgPods)).To(Equal(map[string]string{"tier": "db", "mesh": "on", "cost-centre": "dbaas"}))
		all := set.Labels(All)
		all["cost-centre"] = "changed"
		Expect(set.Labels(All)).To(HaveKeyWithValue("cost-centre", "dbaas"))
	})

	It("does not keep a reference to the rules", func() {
		labels := map[string]string{"a": "1"}
		compiled, _ := Compile([]Rule{{Targets: []Target{All}, Labels: labels}}, Targets)
		labels["a"] = "changed"
		Expect(compiled.Labels(All)).To(HaveKeyWithValue("a", "1"))
	})
})

var _ = Describe("Stacking the layers of an object", func() {
	compile := func(rules ...Rule) Set {
		set, found := Compile(rules, Targets)
		Expect(found).To(BeEmpty())
		return set
	}

	It("gives what the resource gives when no layer has rules, as before", func() {
		layers := Layers{}
		resource := map[string]string{"team": "payments"}
		Expect(layers.Labels(PgPods, resource)).To(Equal(resource))
		Expect(layers.Annotations(PgPods, nil)).To(BeEmpty())
		Expect(layers.Reaches(PgPods)).To(BeFalse())
	})

	It("stacks defaults, resource, own rules and mandatory values, the highest winning", func() {
		layers := Layers{
			Defaults: compile(Rule{
				Targets: []Target{All},
				Labels:  map[string]string{"d": "default", "r": "default", "o": "default", "m": "default"},
			}),
			Own: compile(Rule{
				Targets: []Target{PgPods},
				Labels:  map[string]string{"o": "own", "m": "own"},
			}),
			Mandatory: compile(Rule{
				Targets: []Target{AllPods},
				Labels:  map[string]string{"m": "mandatory"},
			}),
		}
		resource := map[string]string{"r": "resource", "o": "resource", "m": "resource"}
		Expect(layers.Labels(PgPods, resource)).To(Equal(map[string]string{
			"d": "default",   // nothing above sets it
			"r": "resource",  // the resource over the default
			"o": "own",       // the own rules over the resource
			"m": "mandatory", // the mandatory value over everything
		}))
		// Another kind: the own rule and the allPods mandatory value do not apply
		Expect(layers.Labels(Services, resource)).To(Equal(map[string]string{
			"d": "default", "r": "resource", "o": "resource", "m": "resource",
		}))
	})

	It("stacks the annotations the same way, apart from the labels", func() {
		layers := Layers{
			Defaults:  compile(Rule{Targets: []Target{All}, Annotations: map[string]string{"d": "default", "r": "default"}}),
			Own:       compile(Rule{Targets: []Target{Secrets}, Annotations: map[string]string{"o": "own", "m": "own"}}),
			Mandatory: compile(Rule{Targets: []Target{All}, Annotations: map[string]string{"m": "mandatory"}}),
		}
		Expect(layers.Annotations(Secrets, map[string]string{"r": "resource", "o": "resource"})).To(Equal(
			map[string]string{"d": "default", "r": "resource", "o": "own", "m": "mandatory"}))
		Expect(layers.Labels(Secrets, nil)).To(BeEmpty())
	})

	It("lets a mandatory value on all win over a default on the kind", func() {
		layers := Layers{
			Defaults:  compile(Rule{Targets: []Target{PgPods}, Labels: map[string]string{"x": "default"}}),
			Mandatory: compile(Rule{Targets: []Target{All}, Labels: map[string]string{"x": "mandatory"}}),
		}
		Expect(layers.Labels(PgPods, nil)).To(Equal(map[string]string{"x": "mandatory"}))
	})

	It("keeps labels and annotations apart", func() {
		layers := Layers{Own: compile(Rule{
			Targets: []Target{All},
			Labels:  map[string]string{"x": "label"}, Annotations: map[string]string{"x": "annotation"},
		})}
		Expect(layers.Labels(Secrets, nil)).To(Equal(map[string]string{"x": "label"}))
		Expect(layers.Annotations(Secrets, nil)).To(Equal(map[string]string{"x": "annotation"}))
	})

	It("tells whether any layer reaches a kind", func() {
		Expect(Layers{Defaults: compile(Rule{
			Targets:     []Target{PoolerPods},
			Annotations: map[string]string{"a": "1"},
		})}.Reaches(PoolerPods)).To(BeTrue())
		Expect(Layers{Mandatory: compile(Rule{
			Targets: []Target{AllPods},
			Labels:  map[string]string{"a": "1"},
		})}.Reaches(PoolerPods)).To(BeTrue())
		Expect(Layers{Own: compile(Rule{
			Targets: []Target{PgPods},
			Labels:  map[string]string{"a": "1"},
		})}.Reaches(PoolerPods)).To(BeFalse())
	})

	It("returns a new map, leaving the resource's alone", func() {
		resource := map[string]string{"a": "1"}
		layers := Layers{Own: compile(Rule{Targets: []Target{All}, Labels: map[string]string{"b": "2"}})}
		result := layers.Labels(All, resource)
		result["a"] = "changed"
		Expect(resource).To(Equal(map[string]string{"a": "1"}))
	})
})
