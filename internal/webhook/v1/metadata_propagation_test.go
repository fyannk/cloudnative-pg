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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/internal/configuration"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/metadatarules"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The webhook checks the metadataPropagation rules of a resource: an invalid
// rule is refused with the path of the problem, a key the operator sets
// itself is only warned about, and the values MANDATORY_METADATA replaces
// are named once, for the most general kind replacing them
var _ = Describe("The metadataPropagation rules of a Cluster", func() {
	rule := func(labels, annotations map[string]string, targets ...apiv1.MetadataPropagationTarget,
	) apiv1.MetadataPropagationRule {
		return apiv1.MetadataPropagationRule{Targets: targets, Labels: labels, Annotations: annotations}
	}
	mandatory := func(yaml string) {
		configuration.Current.MandatoryMetadata = yaml
		Expect(configuration.Current.ParseDefaultAndMandatoryMetadata()).To(Succeed())
	}
	replaced := func(kind, key, value, imposed, target string) string {
		return fmt.Sprintf("%s %q: %q set on this Cluster is replaced with %q by MANDATORY_METADATA on %s",
			kind, key, value, imposed, target)
	}

	BeforeEach(func() {
		previous := configuration.Current
		configuration.Current = configuration.NewConfiguration()
		DeferCleanup(func() { configuration.Current = previous })
	})

	Context("validation", func() {
		It("accepts no rule, and valid rules", func() {
			errs, warnings := validateMetadataPropagation(nil, apiv1.ClusterMetadataTargets)
			Expect(errs).To(BeEmpty())
			Expect(warnings).To(BeEmpty())
			errs, warnings = validateMetadataPropagation([]apiv1.MetadataPropagationRule{
				rule(map[string]string{"team": "payments"}, map[string]string{"note": "a,b\nc"}, "pgPods", "services"),
			}, apiv1.ClusterMetadataTargets)
			Expect(errs).To(BeEmpty())
			Expect(warnings).To(BeEmpty())
		})

		DescribeTable("refuses an invalid rule with the path of the problem",
			func(r apiv1.MetadataPropagationRule, path string) {
				errs, _ := validateMetadataPropagation([]apiv1.MetadataPropagationRule{
					rule(map[string]string{"ok": "1"}, nil, "all"), r,
				}, apiv1.ClusterMetadataTargets)
				Expect(errs).To(HaveLen(1))
				Expect(errs[0].Field).To(Equal(path))
			},
			Entry("no target", rule(map[string]string{"a": "1"}, nil),
				"spec.metadataPropagation[1].targets"),
			Entry("a target of another resource", rule(nil, nil, "pgPods", "poolerPods"),
				"spec.metadataPropagation[1].targets[1]"),
			Entry("an unknown target", rule(nil, nil, "nodes"),
				"spec.metadataPropagation[1].targets[0]"),
			Entry("an invalid label key", rule(map[string]string{"my team": "x"}, nil, "all"),
				"spec.metadataPropagation[1].labels[my team]"),
			Entry("an invalid label value", rule(map[string]string{"team": "a,b"}, nil, "all"),
				"spec.metadataPropagation[1].labels[team]"),
			Entry("an invalid annotation key", rule(nil, map[string]string{"": "x"}, "all"),
				"spec.metadataPropagation[1].annotations[]"),
			Entry("a key given another value on the same target", rule(map[string]string{"ok": "2"}, nil, "all"),
				"spec.metadataPropagation[1].labels[ok]"),
		)

		It("warns about the keys the operator sets itself, without refusing them", func() {
			errs, warnings := validateMetadataPropagation([]apiv1.MetadataPropagationRule{
				rule(map[string]string{"cnpg.io/podRole": "x"}, map[string]string{"app.kubernetes.io/name": "y"}, "pgPods"),
			}, apiv1.ClusterMetadataTargets)
			Expect(errs).To(BeEmpty())
			Expect([]string(warnings)).To(Equal([]string{
				`spec.metadataPropagation[0].labels[cnpg.io/podRole]: set by the operator, never applied`,
				`spec.metadataPropagation[0].annotations[app.kubernetes.io/name]: set by the operator, never applied`,
			}))
		})

		It("refuses an invalid rule when a Cluster is created", func(ctx SpecContext) {
			cluster := &apiv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "payments"},
				Spec: apiv1.ClusterSpec{MetadataPropagation: []apiv1.MetadataPropagationRule{
					rule(map[string]string{"team": "a b"}, nil, "pgPods"),
				}},
			}
			_, err := (&ClusterCustomValidator{}).ValidateCreate(ctx, cluster)
			Expect(err).To(MatchError(ContainSubstring("spec.metadataPropagation[0].labels[team]")))
		})

		It("is part of the validation and the warnings of a Cluster", func() {
			cluster := &apiv1.Cluster{Spec: apiv1.ClusterSpec{MetadataPropagation: []apiv1.MetadataPropagationRule{
				rule(map[string]string{"team": "a b", "cnpg.io/x": "1"}, nil, "pgPods"),
			}}}
			v := &ClusterCustomValidator{}
			Expect(v.validateMetadataPropagation(cluster)).To(HaveLen(1))
			Expect(v.getAdmissionWarnings(cluster)).To(ContainElement(
				ContainSubstring(`spec.metadataPropagation[0].labels[cnpg.io/x]`)))
		})
	})

	Context("values replaced by MANDATORY_METADATA", func() {
		withLabels := func(labels map[string]string, rules ...apiv1.MetadataPropagationRule) *apiv1.Cluster {
			return &apiv1.Cluster{Spec: apiv1.ClusterSpec{
				InheritedMetadata:   &apiv1.EmbeddedObjectMetadata{Labels: labels},
				MetadataPropagation: rules,
			}}
		}
		warningsOf := func(cluster *apiv1.Cluster) []string {
			return getReplacedByMandatoryWarnings(metadatarules.ClusterResource, apiv1.ClusterMetadataTargets,
				cluster.GetClusterLabelsFor, cluster.GetClusterAnnotationsFor)
		}

		It("says nothing without mandatory metadata, whatever the Cluster sets", func() {
			Expect(warningsOf(withLabels(map[string]string{"cost-centre": "marketing"}))).To(BeEmpty())
		})

		It("says nothing when the Cluster does not set the key, or sets the same value", func() {
			mandatory(`[{"targets": ["all"], "labels": {"cost-centre": "dbaas"}}]`)
			Expect(warningsOf(withLabels(nil))).To(BeEmpty())
			Expect(warningsOf(withLabels(map[string]string{"cost-centre": "dbaas"}))).To(BeEmpty())
		})

		It("only sees the mandatory rules reaching the objects of a Cluster", func() {
			mandatory(`[{"resources": ["Pooler"], "targets": ["all"], "labels": {"cost-centre": "pooler"}},
				{"resources": ["Cluster"], "targets": ["services"], "labels": {"cost-centre": "dbaas"}}]`)
			Expect(warningsOf(withLabels(map[string]string{"cost-centre": "marketing"}))).To(Equal([]string{
				replaced("label", "cost-centre", "marketing", "dbaas", "services"),
			}))
		})

		It("reports a value replaced on every object once, for all", func() {
			mandatory(`[{"targets": ["all"], "labels": {"cost-centre": "dbaas"}}]`)
			Expect(warningsOf(withLabels(map[string]string{"cost-centre": "marketing"}))).To(Equal([]string{
				replaced("label", "cost-centre", "marketing", "dbaas", "all"),
			}))
		})

		It("reports a kind whose mandatory value differs from that of the kind covering it", func() {
			mandatory(`[{"targets": ["all"], "labels": {"bla": "0"}}, {"targets": ["allPods"], "labels": {"bla": "pods"}},
				{"targets": ["pgPods"], "labels": {"bla": "1"}}, {"targets": ["majorUpgradePods"], "labels": {"bla": "3"}}]`)
			Expect(warningsOf(withLabels(map[string]string{"bla": "mine"}))).To(Equal([]string{
				replaced("label", "bla", "mine", "0", "all"),
				replaced("label", "bla", "mine", "pods", "allPods"),
				replaced("label", "bla", "mine", "1", "pgPods"),
				replaced("label", "bla", "mine", "3", "majorUpgradePods"),
			}))
		})

		It("compares each kind with what the Cluster's rules set on that kind", func() {
			mandatory(`[{"targets": ["services"], "labels": {"tier": "data"}},
				{"targets": ["pgPods"], "annotations": {"example.com/owner": "platform"}}]`)
			cluster := withLabels(nil, rule(map[string]string{"tier": "frontend"},
				map[string]string{"example.com/owner": "team-a"}, "pgPods"))
			Expect(warningsOf(cluster)).To(Equal([]string{
				replaced("annotation", "example.com/owner", "team-a", "platform", "pgPods"),
			}))
		})

		It("sees the Cluster's value whatever its source", func() {
			configuration.Current.InheritedLabels = []string{"env"}
			mandatory(`[{"targets": ["all"], "labels": {"env": "prod", "team": "platform"},
				"annotations": {"note": "x"}}]`)
			cluster := &apiv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"env": "dev"}},
				Spec: apiv1.ClusterSpec{
					InheritedMetadata: &apiv1.EmbeddedObjectMetadata{Annotations: map[string]string{"note": "y"}},
					MetadataPropagation: []apiv1.MetadataPropagationRule{
						rule(map[string]string{"team": "payments"}, nil, "pgPods"),
					},
				},
			}
			Expect(warningsOf(cluster)).To(ConsistOf(
				replaced("label", "env", "dev", "prod", "all"),
				replaced("annotation", "note", "y", "x", "all"),
				replaced("label", "team", "payments", "platform", "pgPods"),
			))
		})

		It("is part of the admission warnings of a Cluster", func() {
			mandatory(`[{"targets": ["all"], "labels": {"cost-centre": "dbaas"}}]`)
			v := &ClusterCustomValidator{}
			Expect(v.getAdmissionWarnings(withLabels(map[string]string{"cost-centre": "marketing"}))).To(
				ContainElement(replaced("label", "cost-centre", "marketing", "dbaas", "all")))
		})

		It("is part of the admission warnings of a Cluster for the rules naming it only", func() {
			mandatory(`[{"resources": ["Pooler"], "targets": ["all"], "labels": {"cost-centre": "pooler"}},
				{"resources": ["Cluster"], "targets": ["pgPods"], "labels": {"cost-centre": "dbaas"}}]`)
			v := &ClusterCustomValidator{}
			Expect(v.getAdmissionWarnings(withLabels(map[string]string{"cost-centre": "marketing"}))).To(ConsistOf(
				replaced("label", "cost-centre", "marketing", "dbaas", "pgPods")))
		})
	})
})

var _ = Describe("The metadataPropagation rules of a Pooler", func() {
	rule := func(labels, annotations map[string]string, targets ...apiv1.MetadataPropagationTarget,
	) apiv1.MetadataPropagationRule {
		return apiv1.MetadataPropagationRule{Targets: targets, Labels: labels, Annotations: annotations}
	}
	mandatory := func(yaml string) {
		configuration.Current.MandatoryMetadata = yaml
		Expect(configuration.Current.ParseDefaultAndMandatoryMetadata()).To(Succeed())
	}
	replaced := func(kind, key, value, imposed, target string) string {
		return fmt.Sprintf("%s %q: %q set on this Pooler is replaced with %q by MANDATORY_METADATA on %s",
			kind, key, value, imposed, target)
	}
	pooler := func(template map[string]string, rules ...apiv1.MetadataPropagationRule) *apiv1.Pooler {
		result := &apiv1.Pooler{Spec: apiv1.PoolerSpec{MetadataPropagation: rules}}
		if template != nil {
			result.Spec.Template = &apiv1.PodTemplateSpec{ObjectMeta: apiv1.Metadata{Labels: template}}
		}
		return result
	}
	warningsOf := func(pooler *apiv1.Pooler) []string {
		return getReplacedByMandatoryWarnings(metadatarules.PoolerResource, apiv1.PoolerMetadataTargets,
			pooler.GetPoolerLabelsFor, pooler.GetPoolerAnnotationsFor)
	}

	BeforeEach(func() {
		previous := configuration.Current
		configuration.Current = configuration.NewConfiguration()
		DeferCleanup(func() { configuration.Current = previous })
	})

	Context("validation", func() {
		It("accepts every target of a Pooler", func() {
			errs, warnings := validateMetadataPropagation([]apiv1.MetadataPropagationRule{
				rule(map[string]string{"team": "payments"}, nil,
					"all", "poolerPods", "poolerDeployments", "services", "secrets"),
			}, apiv1.PoolerMetadataTargets)
			Expect(errs).To(BeEmpty())
			Expect(warnings).To(BeEmpty())
		})

		DescribeTable("refuses the targets of a Cluster",
			func(target apiv1.MetadataPropagationTarget) {
				errs, _ := validateMetadataPropagation([]apiv1.MetadataPropagationRule{
					rule(map[string]string{"team": "payments"}, nil, "poolerPods", target),
				}, apiv1.PoolerMetadataTargets)
				Expect(errs).To(HaveLen(1))
				Expect(errs[0].Field).To(Equal("spec.metadataPropagation[0].targets[1]"))
			},
			Entry("allPods", apiv1.MetadataPropagationTarget("allPods")),
			Entry("pgPods", apiv1.MetadataPropagationTarget("pgPods")),
			Entry("majorUpgradePods", apiv1.MetadataPropagationTarget("majorUpgradePods")),
			Entry("persistentVolumeClaims", apiv1.MetadataPropagationTarget("persistentVolumeClaims")),
			Entry("podDisruptionBudgets", apiv1.MetadataPropagationTarget("podDisruptionBudgets")),
			Entry("backups", apiv1.MetadataPropagationTarget("backups")),
		)

		It("is part of the validation and the warnings of a Pooler", func() {
			p := pooler(nil, rule(map[string]string{"team": "a b", "cnpg.io/poolerName": "x"}, nil, "poolerPods"))
			v := &PoolerCustomValidator{}
			Expect(v.validate(p)).To(ContainElement(HaveField("Field", "spec.metadataPropagation[0].labels[team]")))
			Expect(v.getAdmissionWarnings(p)).To(ContainElement(
				ContainSubstring(`spec.metadataPropagation[0].labels[cnpg.io/poolerName]`)))
		})
	})

	Context("values replaced by MANDATORY_METADATA", func() {
		It("says nothing without mandatory metadata, or when the Pooler agrees", func() {
			Expect(warningsOf(pooler(map[string]string{"team": "a"}))).To(BeEmpty())
			mandatory(`[{"targets": ["all"], "labels": {"team": "a"}}]`)
			Expect(warningsOf(pooler(map[string]string{"team": "a"}))).To(BeEmpty())
		})

		It("only sees the mandatory rules reaching the objects of a Pooler", func() {
			mandatory(`[{"resources": ["Cluster"], "targets": ["all"], "labels": {"team": "databases"}},
				{"resources": ["Pooler"], "targets": ["allPods"], "labels": {"team": "platform"}}]`)
			Expect(warningsOf(pooler(map[string]string{"team": "mine"}))).To(Equal([]string{
				replaced("label", "team", "mine", "platform", "poolerPods"),
			}))
		})

		It("are part of the admission warnings of a Pooler, for the rules naming it only", func() {
			mandatory(`[{"resources": ["Cluster"], "targets": ["all"], "labels": {"team": "databases"}},
				{"resources": ["Pooler"], "targets": ["poolerPods"], "labels": {"team": "platform"}}]`)
			Expect((&PoolerCustomValidator{}).getAdmissionWarnings(pooler(map[string]string{"team": "mine"}))).To(
				ConsistOf(replaced("label", "team", "mine", "platform", "poolerPods")))
		})

		It("reports a value of the pod template on the Pooler Pods", func() {
			mandatory(`[{"targets": ["all"], "labels": {"team": "platform"}}]`)
			Expect(warningsOf(pooler(map[string]string{"team": "mine"}))).To(Equal([]string{
				replaced("label", "team", "mine", "platform", "poolerPods"),
			}))
		})

		It("reports a value of a rule for all once", func() {
			mandatory(`[{"targets": ["all"], "labels": {"team": "platform"}}]`)
			Expect(warningsOf(pooler(nil, rule(map[string]string{"team": "mine"}, nil, "all")))).To(Equal([]string{
				replaced("label", "team", "mine", "platform", "all"),
			}))
		})

		It("reports a value of a rule for the Deployments", func() {
			mandatory(`[{"targets": ["poolerDeployments"], "annotations": {"example.com/owner": "platform"}}]`)
			p := pooler(nil, rule(nil, map[string]string{"example.com/owner": "mine"}, "poolerDeployments"))
			Expect(warningsOf(p)).To(Equal([]string{
				replaced("annotation", "example.com/owner", "mine", "platform", "poolerDeployments"),
			}))
		})

		It("is part of the admission warnings of a Pooler", func() {
			mandatory(`[{"targets": ["allPods"], "labels": {"team": "platform"}}]`)
			v := &PoolerCustomValidator{}
			Expect(v.getAdmissionWarnings(pooler(map[string]string{"team": "mine"}))).To(
				ContainElement(replaced("label", "team", "mine", "platform", "poolerPods")))
		})
	})
})
