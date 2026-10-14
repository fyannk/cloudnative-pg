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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/cloudnative-pg/cloudnative-pg/internal/configuration"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/metadatarules"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// rule builds a metadata propagation rule
func rule(labels, annotations map[string]string, targets ...MetadataPropagationTarget) MetadataPropagationRule {
	return MetadataPropagationRule{Targets: targets, Labels: labels, Annotations: annotations}
}

// The metadata the operator sets on every object of a Cluster goes through
// GetInheritedLabelsFor and GetInheritedAnnotationsFor: first, they must give
// exactly what the operator gave before the rules existed, then each rule
// must land where, and only where, it is meant to.
var _ = Describe("The metadata of the objects of a Cluster", func() {
	var previous *configuration.Data

	settings := func(defaults, mandatory string) {
		configuration.Current.DefaultMetadata = defaults
		configuration.Current.MandatoryMetadata = mandatory
		Expect(configuration.Current.ParseDefaultAndMandatoryMetadata()).To(Succeed())
	}

	BeforeEach(func() {
		previous = configuration.Current
		configuration.Current = configuration.NewConfiguration()
		DeferCleanup(func() { configuration.Current = previous })
	})

	// stockObject builds the metadata of an object as the operator did
	// before, with the functions it used then
	stockObject := func(cluster *Cluster, existing metav1.ObjectMeta) metav1.ObjectMeta {
		meta := *existing.DeepCopy()
		utils.InheritAnnotations(&meta, cluster.Annotations, cluster.GetFixedInheritedAnnotations(), configuration.Current)
		utils.InheritLabels(&meta, cluster.Labels, cluster.GetFixedInheritedLabels(), configuration.Current)
		utils.LabelClusterName(&meta, cluster.GetName())
		return meta
	}

	Context("without rules, from the Cluster or the operator", func() {
		DescribeTable("gives every kind of object what the operator gave before",
			func(inheritedLabels, inheritedAnnotations []string, cluster Cluster, existing metav1.ObjectMeta) {
				configuration.Current.InheritedLabels = inheritedLabels
				configuration.Current.InheritedAnnotations = inheritedAnnotations
				Expect(configuration.Current.ParseDefaultAndMandatoryMetadata()).To(Succeed())

				expected := stockObject(&cluster, existing)
				for _, target := range metadatarules.Targets {
					meta := *existing.DeepCopy()
					cluster.SetInheritedDataFor(&meta, target)
					Expect(meta).To(Equal(expected), string(target))
				}

				// SetInheritedData, used by every object without a kind,
				// gives the same
				meta := *existing.DeepCopy()
				cluster.SetInheritedData(&meta)
				Expect(meta).To(Equal(expected))
			},
			Entry("a Cluster with no metadata at all",
				nil, nil,
				Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c"}},
				metav1.ObjectMeta{}),
			Entry("inheritedMetadata only",
				nil, nil,
				Cluster{
					ObjectMeta: metav1.ObjectMeta{Name: "c", Labels: map[string]string{"not": "inherited"}},
					Spec: ClusterSpec{InheritedMetadata: &EmbeddedObjectMetadata{
						Labels:      map[string]string{"team": "payments"},
						Annotations: map[string]string{"note": "x"},
					}},
				},
				metav1.ObjectMeta{}),
			Entry("the Cluster's own metadata selected by INHERITED_*, with globs",
				[]string{"env", "example.com/*"}, []string{"categories"},
				Cluster{ObjectMeta: metav1.ObjectMeta{
					Name:        "c",
					Labels:      map[string]string{"env": "prod", "example.com/a": "1", "other": "x"},
					Annotations: map[string]string{"categories": "db", "other": "x"},
				}},
				metav1.ObjectMeta{}),
			Entry("the Cluster's own label winning over inheritedMetadata on the same key",
				[]string{"team"}, nil,
				Cluster{
					ObjectMeta: metav1.ObjectMeta{Name: "c", Labels: map[string]string{"team": "own"}},
					Spec: ClusterSpec{InheritedMetadata: &EmbeddedObjectMetadata{
						Labels: map[string]string{"team": "inherited"},
					}},
				},
				metav1.ObjectMeta{}),
			Entry("an object that already has metadata, kept and updated",
				[]string{"env"}, nil,
				Cluster{
					ObjectMeta: metav1.ObjectMeta{Name: "c", Labels: map[string]string{"env": "prod"}},
					Spec: ClusterSpec{InheritedMetadata: &EmbeddedObjectMetadata{
						Labels: map[string]string{"team": "payments"},
					}},
				},
				metav1.ObjectMeta{
					Labels:      map[string]string{"cnpg.io/podRole": "instance", "env": "old", "third-party": "x"},
					Annotations: map[string]string{"third-party": "y"},
				}),
			Entry("inheritedMetadata on a key the operator sets, overwritten as before",
				nil, nil,
				Cluster{
					ObjectMeta: metav1.ObjectMeta{Name: "c"},
					Spec: ClusterSpec{InheritedMetadata: &EmbeddedObjectMetadata{
						Labels: map[string]string{"app.kubernetes.io/name": "custom"},
					}},
				},
				metav1.ObjectMeta{Labels: map[string]string{"app.kubernetes.io/name": "postgresql"}}),
		)

		It("keeps the operator version annotation on every kind of object", func() {
			cluster := Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c"}}
			for _, target := range metadatarules.Targets {
				Expect(cluster.GetInheritedAnnotationsFor(target)).To(
					HaveKey(utils.OperatorVersionAnnotationName), string(target))
			}
		})
	})

	Context("with metadataPropagation rules", func() {
		var cluster Cluster

		BeforeEach(func() {
			configuration.Current.InheritedLabels = []string{"env"}
			cluster = Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "payments", Labels: map[string]string{"env": "prod"}},
				Spec: ClusterSpec{
					InheritedMetadata: &EmbeddedObjectMetadata{
						Labels:      map[string]string{"team": "payments", "tier": "inherited"},
						Annotations: map[string]string{"note": "inherited"},
					},
					MetadataPropagation: []MetadataPropagationRule{
						rule(map[string]string{"tier": "pods", "env": "pods-env"},
							map[string]string{"vault.hashicorp.com/agent-inject": "true"}, "pgPods"),
						rule(nil, map[string]string{"external-dns.kubernetes.io/hostname": "db.example.com"},
							"services"),
						rule(map[string]string{"velero.io/exclude-from-backup": "true"}, nil,
							"persistentVolumeClaims"),
						rule(nil, map[string]string{"replicator.v1.mittwald.de/replicate-to": "apps,batch"}, "secrets"),
						rule(map[string]string{"tier": "upgrade"}, map[string]string{"sidecar.istio.io/inject": "false"},
							"majorUpgradePods"),
						rule(nil, map[string]string{"example.com/note": "pdb"}, "podDisruptionBudgets"),
					},
				},
			}
		})

		DescribeTable("put each rule on its kind only, on top of the Cluster's metadata",
			func(target metadatarules.Target, labels, annotations map[string]string) {
				Expect(cluster.GetInheritedLabelsFor(target)).To(Equal(labels))
				got := cluster.GetInheritedAnnotationsFor(target)
				delete(got, utils.OperatorVersionAnnotationName)
				Expect(got).To(Equal(annotations))
			},
			Entry("the instance Pods, where a rule wins even over INHERITED_LABELS",
				metadatarules.PgPods,
				map[string]string{"env": "pods-env", "team": "payments", "tier": "pods"},
				map[string]string{"note": "inherited", "vault.hashicorp.com/agent-inject": "true"}),
			Entry("the major upgrade Pods, apart from the PostgreSQL Pods",
				metadatarules.MajorUpgradePods,
				map[string]string{"env": "prod", "team": "payments", "tier": "upgrade"},
				map[string]string{"note": "inherited", "sidecar.istio.io/inject": "false"}),
			Entry("the Services",
				metadatarules.Services,
				map[string]string{"env": "prod", "team": "payments", "tier": "inherited"},
				map[string]string{"note": "inherited", "external-dns.kubernetes.io/hostname": "db.example.com"}),
			Entry("the PVCs",
				metadatarules.PersistentVolumeClaims,
				map[string]string{
					"env": "prod", "team": "payments", "tier": "inherited", "velero.io/exclude-from-backup": "true",
				},
				map[string]string{"note": "inherited"}),
			Entry("the Secrets, with a value holding a comma",
				metadatarules.Secrets,
				map[string]string{"env": "prod", "team": "payments", "tier": "inherited"},
				map[string]string{"note": "inherited", "replicator.v1.mittwald.de/replicate-to": "apps,batch"}),
			Entry("the PodDisruptionBudgets",
				metadatarules.PodDisruptionBudgets,
				map[string]string{"env": "prod", "team": "payments", "tier": "inherited"},
				map[string]string{"note": "inherited", "example.com/note": "pdb"}),
			Entry("every other object",
				metadatarules.All,
				map[string]string{"env": "prod", "team": "payments", "tier": "inherited"},
				map[string]string{"note": "inherited"}),
		)

		It("layer all, allPods and a kind of Pod, the most specific winning", func() {
			cluster.Spec.MetadataPropagation = []MetadataPropagationRule{
				rule(map[string]string{"bla": "1"}, nil, "pgPods"),
				rule(map[string]string{"bla": "pods", "mesh": "on"}, nil, "allPods"),
				rule(map[string]string{"bla": "0", "cost-centre": "x"}, nil, "all"),
			}
			Expect(cluster.GetInheritedLabelsFor(metadatarules.PgPods)).To(
				SatisfyAll(HaveKeyWithValue("bla", "1"), HaveKeyWithValue("mesh", "on"),
					HaveKeyWithValue("cost-centre", "x")))
			Expect(cluster.GetInheritedLabelsFor(metadatarules.MajorUpgradePods)).To(HaveKeyWithValue("bla", "pods"))
			Expect(cluster.GetInheritedLabelsFor(metadatarules.Services)).To(
				SatisfyAll(HaveKeyWithValue("bla", "0"), Not(HaveKey("mesh"))))
		})

		It("never take the keys the operator sets itself from a rule", func() {
			cluster.Spec.MetadataPropagation = []MetadataPropagationRule{rule(
				map[string]string{
					"cnpg.io/podRole": "fake", "alpha.cnpg.io/x": "fake",
					"app.kubernetes.io/name": "fake", "app.kubernetes.io/part-of": "billing",
				},
				map[string]string{utils.OperatorVersionAnnotationName: "fake"}, "pgPods")}
			labels := cluster.GetInheritedLabelsFor(metadatarules.PgPods)
			Expect(labels).NotTo(HaveKey("cnpg.io/podRole"))
			Expect(labels).NotTo(HaveKey("alpha.cnpg.io/x"))
			Expect(labels).NotTo(HaveKey("app.kubernetes.io/name"))
			Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/part-of", "billing"))
			Expect(cluster.GetInheritedAnnotationsFor(metadatarules.PgPods)).NotTo(
				HaveKeyWithValue(utils.OperatorVersionAnnotationName, "fake"))
		})

		It("apply the valid part of invalid rules, which the webhook refuses", func() {
			cluster.Spec.MetadataPropagation = []MetadataPropagationRule{
				rule(map[string]string{"ok": "1", "bad": "a b"}, nil, "pgPods", "poolerPods"),
			}
			labels := cluster.GetInheritedLabelsFor(metadatarules.PgPods)
			Expect(labels).To(HaveKeyWithValue("ok", "1"))
			Expect(labels).NotTo(HaveKey("bad"))
			Expect(cluster.GetInheritedLabelsFor(metadatarules.PoolerPods)).NotTo(HaveKey("ok"))
		})

		It("leave the Cluster spec untouched", func() {
			for _, target := range metadatarules.Targets {
				_ = cluster.GetInheritedLabelsFor(target)
				meta := metav1.ObjectMeta{Labels: map[string]string{"x": "y"}}
				cluster.SetInheritedDataFor(&meta, target)
			}
			Expect(cluster.Spec.InheritedMetadata.Labels).To(Equal(map[string]string{"team": "payments", "tier": "inherited"}))
			Expect(cluster.Spec.MetadataPropagation[0].Labels).To(Equal(map[string]string{"tier": "pods", "env": "pods-env"}))
			Expect(cluster.Labels).To(Equal(map[string]string{"env": "prod"}))
		})
	})

	Context("with default and mandatory metadata", func() {
		var cluster Cluster

		BeforeEach(func() {
			configuration.Current.InheritedLabels = []string{"env"}
			settings(`
- targets: [all]
  labels: {team: unassigned, env: default-env}
- targets: [pgPods]
  labels: {mesh: default}
  annotations: {example.com/mesh: "off"}
- targets: [allPods]
  labels: {mesh: pods-default}
`, `
- targets: [all]
  labels: {cost-centre: dbaas}
- targets: [pgPods]
  labels: {tier: imposed}
- targets: [majorUpgradePods]
  labels: {tier: upgrade}
- targets: [allPods]
  labels: {cost-centre: pods}
- targets: [services]
  annotations: {example.com/owner: platform}
`)
			cluster = Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "payments", Labels: map[string]string{"env": "prod"}},
				Spec: ClusterSpec{
					InheritedMetadata: &EmbeddedObjectMetadata{
						Labels: map[string]string{"team": "payments", "cost-centre": "marketing"},
					},
					MetadataPropagation: []MetadataPropagationRule{
						rule(map[string]string{"tier": "pods", "mesh": "cluster"},
							map[string]string{"example.com/mesh": "on"}, "pgPods"),
						rule(nil, map[string]string{"example.com/owner": "team-a"}, "services"),
					},
				},
			}
		})

		DescribeTable("stack the defaults, the Cluster's metadata and rules, and the mandatory values",
			func(target metadatarules.Target, labels, annotations map[string]string) {
				Expect(cluster.GetInheritedLabelsFor(target)).To(Equal(labels))
				got := cluster.GetInheritedAnnotationsFor(target)
				delete(got, utils.OperatorVersionAnnotationName)
				Expect(got).To(Equal(annotations))
			},
			Entry("the instance Pods", metadatarules.PgPods,
				map[string]string{
					"team":        "payments", // inheritedMetadata over the default
					"env":         "prod",     // INHERITED_LABELS over the default
					"mesh":        "cluster",  // the Cluster's rule over the default of the kind
					"tier":        "imposed",  // mandatory over the Cluster's rule
					"cost-centre": "pods",     // mandatory of allPods over that of all
				},
				map[string]string{"example.com/mesh": "on"}),
			Entry("the major upgrade Pods: allPods and their own, not those of the PostgreSQL Pods",
				metadatarules.MajorUpgradePods,
				map[string]string{
					"team": "payments", "env": "prod", "mesh": "pods-default", "tier": "upgrade", "cost-centre": "pods",
				},
				map[string]string{}),
			Entry("the Services: the mandatory annotation over the Cluster's rule", metadatarules.Services,
				map[string]string{"team": "payments", "env": "prod", "cost-centre": "dbaas"},
				map[string]string{"example.com/owner": "platform"}),
			Entry("every other object: all only", metadatarules.All,
				map[string]string{"team": "payments", "env": "prod", "cost-centre": "dbaas"},
				map[string]string{}),
		)

		It("apply a default when the Cluster does not set the key", func() {
			cluster.Spec.InheritedMetadata = nil
			cluster.Labels = nil
			cluster.Spec.MetadataPropagation = nil
			Expect(cluster.GetInheritedLabelsFor(metadatarules.PgPods)).To(Equal(map[string]string{
				"team": "unassigned", "env": "default-env", "mesh": "default", "cost-centre": "pods", "tier": "imposed",
			}))
			Expect(cluster.GetInheritedAnnotationsFor(metadatarules.PgPods)).To(
				HaveKeyWithValue("example.com/mesh", "off"))
		})

		It("keep what the Cluster asks for apart, for the webhook to compare", func() {
			Expect(cluster.GetClusterLabelsFor(metadatarules.PgPods)).To(Equal(map[string]string{
				"team": "payments", "env": "prod", "cost-centre": "marketing", "tier": "pods", "mesh": "cluster",
			}))
			Expect(cluster.GetClusterLabelsFor(metadatarules.All)).NotTo(HaveKey("tier"))
		})

		It("set them on an object with the cluster label, keeping its other keys", func() {
			meta := metav1.ObjectMeta{Labels: map[string]string{
				"cnpg.io/podRole": "instance", "third-party": "x", "cost-centre": "stale",
			}}
			cluster.SetInheritedDataFor(&meta, metadatarules.PgPods)
			Expect(meta.Labels).To(HaveKeyWithValue("cnpg.io/podRole", "instance"))
			Expect(meta.Labels).To(HaveKeyWithValue("third-party", "x"))
			Expect(meta.Labels).To(HaveKeyWithValue("cost-centre", "pods"))
			Expect(meta.Labels).To(HaveKeyWithValue("cnpg.io/cluster", "payments"))
		})

		It("set the owner with SetInheritedDataAndOwnershipFor", func() {
			cluster.TypeMeta = metav1.TypeMeta{Kind: ClusterKind, APIVersion: SchemeGroupVersion.String()}
			cluster.UID = "uid"
			var meta metav1.ObjectMeta
			cluster.SetInheritedDataAndOwnershipFor(&meta, metadatarules.Services)
			Expect(meta.OwnerReferences).To(HaveLen(1))
			Expect(meta.OwnerReferences[0].Name).To(Equal("payments"))
			Expect(meta.Annotations).To(HaveKeyWithValue("example.com/owner", "platform"))
		})
	})

	Context("with default and mandatory metadata naming the kinds of resources", func() {
		It("take the rules naming the Cluster, not those of the other resources", func() {
			settings(`[{"resources": ["Pooler"], "targets": ["all"], "labels": {"pooler-default": "x"}},
				{"resources": ["Cluster"], "targets": ["services"], "labels": {"lb-pool": "databases"}}]`,
				`[{"resources": ["Pooler", "Backup"], "targets": ["all"], "annotations": {"pooler-mandatory": "y"}},
				  {"resources": ["Cluster"], "targets": ["all"], "annotations": {"cluster-mandatory": "z"}}]`)
			cluster := &Cluster{ObjectMeta: metav1.ObjectMeta{Name: "payments"}}
			for _, target := range ClusterMetadataTargets {
				Expect(cluster.GetInheritedLabelsFor(target)).NotTo(HaveKey("pooler-default"), string(target))
				Expect(cluster.GetInheritedAnnotationsFor(target)).To(HaveKeyWithValue("cluster-mandatory", "z"),
					string(target))
				Expect(cluster.GetInheritedAnnotationsFor(target)).NotTo(HaveKey("pooler-mandatory"), string(target))
			}
			Expect(cluster.GetInheritedLabelsFor(metadatarules.Services)).To(
				Equal(map[string]string{"lb-pool": "databases"}))
			Expect(cluster.GetInheritedLabelsFor(metadatarules.PgPods)).To(BeEmpty())
		})
	})
})

var _ = Describe("Converting metadata propagation rules", func() {
	It("gives nothing for no rule", func() {
		Expect(ToMetadataRules(nil)).To(BeNil())
		Expect(ToMetadataRules([]MetadataPropagationRule{})).To(BeNil())
	})

	It("keeps every target, label and annotation, in order", func() {
		rules := ToMetadataRules([]MetadataPropagationRule{
			rule(map[string]string{"a": "1"}, nil, "pgPods", "services"),
			rule(nil, map[string]string{"b": "2"}, "all"),
		})
		Expect(rules).To(Equal([]metadatarules.Rule{
			{
				Targets: []metadatarules.Target{metadatarules.PgPods, metadatarules.Services},
				Labels:  map[string]string{"a": "1"},
			},
			{Targets: []metadatarules.Target{metadatarules.All}, Annotations: map[string]string{"b": "2"}},
		}))
	})

	It("lists the targets of a Cluster", func() {
		Expect(ClusterMetadataTargets).To(ConsistOf(metadatarules.All, metadatarules.AllPods, metadatarules.PgPods,
			metadatarules.MajorUpgradePods, metadatarules.Services, metadatarules.PersistentVolumeClaims,
			metadatarules.Secrets, metadatarules.PodDisruptionBudgets))
	})
})
