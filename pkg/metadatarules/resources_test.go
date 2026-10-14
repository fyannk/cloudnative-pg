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
	"sigs.k8s.io/yaml"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("The resources", func() {
	It("are all listed, once", func() {
		Expect(Resources).To(ConsistOf(ClusterResource, PoolerResource, ScheduledBackupResource, BackupResource))
	})

	DescribeTable("know the targets of the objects each one creates",
		func(resource Resource, expected []Target) {
			Expect(resource.Targets()).To(Equal(expected))
		},
		Entry("Cluster", ClusterResource, []Target{
			All, AllPods, PgPods, MajorUpgradePods, Services, PersistentVolumeClaims, Secrets,
			PodDisruptionBudgets,
		}),
		Entry("Pooler", PoolerResource, []Target{All, PoolerPods, PoolerDeployments, Services, Secrets}),
		Entry("ScheduledBackup", ScheduledBackupResource, []Target{Backups}),
		Entry("Backup", BackupResource, []Target{VolumeSnapshots}),
		Entry("an unknown resource", Resource("Database"), []Target(nil)),
	)

	It("give each known target to at least one resource", func() {
		for _, target := range Targets {
			covered := false
			for _, resource := range Resources {
				covered = covered || resource.Covers(target)
			}
			Expect(covered).To(BeTrue(), string(target))
		}
	})

	DescribeTable("tell the targets reaching their objects, covering targets included",
		func(resource Resource, expected []Target) {
			Expect(resource.coveringTargets()).To(Equal(expected))
			for _, target := range Targets {
				Expect(resource.Covers(target)).To(Equal(contains(expected, target)), string(target))
			}
		},
		Entry("Cluster: its own targets", ClusterResource, []Target{
			All, AllPods, PgPods, MajorUpgradePods,
			Services, PersistentVolumeClaims, Secrets, PodDisruptionBudgets,
		}),
		Entry("Pooler: allPods covers its Pods", PoolerResource, []Target{
			All, AllPods, PoolerPods,
			PoolerDeployments, Services, Secrets,
		}),
		Entry("ScheduledBackup: all covers its Backups", ScheduledBackupResource, []Target{All, Backups}),
		Entry("Backup: all covers its VolumeSnapshots", BackupResource, []Target{All, VolumeSnapshots}),
		Entry("an unknown resource: none", Resource("Database"), []Target(nil)),
	)
})

func contains(list []Target, target Target) bool {
	for _, item := range list {
		if item == target {
			return true
		}
	}
	return false
}

// operatorRules reads operator rules as the operator configuration gives them
func operatorRules(text string) []OperatorRule {
	var rules []OperatorRule
	Expect(yaml.UnmarshalStrict([]byte(text), &rules)).To(Succeed())
	return rules
}

var _ = Describe("Compiling the operator rules", func() {
	It("accepts no rule at all, and the zero value gives nothing", func() {
		sets, found := CompileOperatorRules(nil)
		Expect(found).To(BeEmpty())
		for _, resource := range Resources {
			Expect(sets.For(resource).IsEmpty()).To(BeTrue())
		}
		Expect(Sets(nil).For(ClusterResource).IsEmpty()).To(BeTrue())
		Expect(Sets(nil).For(ClusterResource).Labels(PgPods)).To(BeNil())
	})

	It("reads the resources next to the fields of a rule", func() {
		rules := operatorRules("- resources: [Pooler]\n  targets: [services]\n  labels: {a: b}\n  annotations: {c: d}\n")
		Expect(rules).To(Equal([]OperatorRule{{
			Resources: []Resource{PoolerResource},
			Rule: Rule{
				Targets: []Target{Services}, Labels: map[string]string{"a": "b"},
				Annotations: map[string]string{"c": "d"},
			},
		}}))
		var plain []OperatorRule
		Expect(yaml.UnmarshalStrict([]byte("- targets: [all]\n"), &plain)).To(Succeed())
		Expect(plain[0].Resources).To(BeNil())
	})

	It("applies a rule naming no resource to the objects of every resource it reaches", func() {
		sets, found := CompileOperatorRules(operatorRules(
			"- targets: [all]\n  labels: {cost-centre: dbaas}\n" +
				"- targets: [services]\n  annotations: {owner: platform}\n" +
				"- targets: [pgPods]\n  labels: {tier: database}\n"))
		Expect(found).To(BeEmpty())
		for _, resource := range Resources {
			for _, target := range resource.Targets() {
				Expect(sets.For(resource).Labels(target)).To(HaveKeyWithValue("cost-centre", "dbaas"),
					"%s %s", resource, target)
			}
		}
		Expect(sets.For(ClusterResource).Annotations(Services)).To(Equal(map[string]string{"owner": "platform"}))
		Expect(sets.For(PoolerResource).Annotations(Services)).To(Equal(map[string]string{"owner": "platform"}))
		Expect(sets.For(ClusterResource).Labels(PgPods)).To(HaveKeyWithValue("tier", "database"))
		Expect(sets.For(PoolerResource).Labels(PoolerPods)).NotTo(HaveKey("tier"))
		Expect(sets.For(BackupResource).Annotations(Services)).To(BeNil())
	})

	It("gives the services of the Clusters and of the Poolers different values", func() {
		sets, found := CompileOperatorRules(operatorRules(
			"- resources: [Cluster]\n  targets: [services]\n  labels: {lb-pool: databases}\n" +
				"- resources: [Pooler]\n  targets: [services]\n  labels: {lb-pool: poolers}\n"))
		Expect(found).To(BeEmpty())
		Expect(sets.For(ClusterResource).Labels(Services)).To(Equal(map[string]string{"lb-pool": "databases"}))
		Expect(sets.For(PoolerResource).Labels(Services)).To(Equal(map[string]string{"lb-pool": "poolers"}))
		Expect(sets.For(ScheduledBackupResource).IsEmpty()).To(BeTrue())
		Expect(sets.For(BackupResource).IsEmpty()).To(BeTrue())
	})

	It("limits a rule to the objects of the resources it names", func() {
		sets, found := CompileOperatorRules(operatorRules(
			"- resources: [Pooler]\n  targets: [all]\n  labels: {component: pooler}\n" +
				"- resources: [Cluster, Pooler]\n  targets: [secrets]\n  annotations: {replicate: \"true\"}\n" +
				"- resources: [Pooler]\n  targets: [allPods]\n  labels: {mesh: \"off\"}\n"))
		Expect(found).To(BeEmpty())
		for _, target := range PoolerResource.Targets() {
			Expect(sets.For(PoolerResource).Labels(target)).To(HaveKeyWithValue("component", "pooler"), string(target))
		}
		Expect(sets.For(ClusterResource).Reaches(All)).To(BeFalse())
		Expect(sets.For(ClusterResource).Labels(PgPods)).To(BeNil())
		Expect(sets.For(ClusterResource).Annotations(Secrets)).To(Equal(map[string]string{"replicate": "true"}))
		Expect(sets.For(PoolerResource).Annotations(Secrets)).To(Equal(map[string]string{"replicate": "true"}))
		Expect(sets.For(PoolerResource).Labels(PoolerPods)).To(HaveKeyWithValue("mesh", "off"))
		Expect(sets.For(ScheduledBackupResource).IsEmpty()).To(BeTrue())
		Expect(sets.For(BackupResource).IsEmpty()).To(BeTrue())
	})

	It("reaches the Backups and VolumeSnapshots through all, on their own resource only", func() {
		sets, found := CompileOperatorRules(operatorRules(
			"- resources: [ScheduledBackup]\n  targets: [all]\n  labels: {kind: scheduled}\n" +
				"- resources: [Backup]\n  targets: [all]\n  labels: {kind: snapshot}\n"))
		Expect(found).To(BeEmpty())
		Expect(sets.For(ScheduledBackupResource).Labels(Backups)).To(Equal(map[string]string{"kind": "scheduled"}))
		Expect(sets.For(BackupResource).Labels(VolumeSnapshots)).To(Equal(map[string]string{"kind": "snapshot"}))
		Expect(sets.For(ClusterResource).IsEmpty()).To(BeTrue())
		Expect(sets.For(PoolerResource).IsEmpty()).To(BeTrue())
	})

	It("takes the most specific target, then the value of the rule naming the resource or not alike", func() {
		sets, found := CompileOperatorRules(operatorRules(
			"- targets: [all]\n  labels: {tier: everything}\n" +
				"- resources: [Pooler]\n  targets: [services]\n  labels: {tier: pooler-services}\n"))
		Expect(found).To(BeEmpty())
		Expect(sets.For(PoolerResource).Labels(Services)).To(Equal(map[string]string{"tier": "pooler-services"}))
		Expect(sets.For(PoolerResource).Labels(PoolerPods)).To(Equal(map[string]string{"tier": "everything"}))
		Expect(sets.For(ClusterResource).Labels(Services)).To(Equal(map[string]string{"tier": "everything"}))
	})

	DescribeTable("refuses an invalid rule, with the path of the problem",
		func(text string, expected ...string) {
			_, found := CompileOperatorRules(operatorRules(text))
			Expect(problems(found)).To(Equal(expected))
		},
		Entry("an unknown resource",
			"- resources: [Database]\n  targets: [all]\n  labels: {a: b}\n",
			"invalid [0].resources[0]"),
		Entry("a resource with the wrong case",
			"- resources: [Cluster, pooler]\n  targets: [all]\n  labels: {a: b}\n",
			"invalid [0].resources[1]"),
		Entry("a target not reaching the objects of the named resource",
			"- resources: [Pooler]\n  targets: [services, pgPods]\n  labels: {a: b}\n",
			"invalid [0].targets[1]"),
		Entry("a target reaching only one of the named resources",
			"- resources: [Cluster, Pooler]\n  targets: [pgPods]\n  labels: {a: b}\n",
			"invalid [0].targets[0]"),
		Entry("the VolumeSnapshots on a Cluster",
			"- resources: [Cluster]\n  targets: [volumeSnapshots]\n  labels: {a: b}\n",
			"invalid [0].targets[0]"),
		Entry("an unknown target, once",
			"- targets: [nodes]\n  labels: {a: b}\n",
			"invalid [0].targets[0]"),
		Entry("no target, once",
			"- resources: [Pooler]\n  labels: {a: b}\n",
			"invalid [0].targets"),
		Entry("an invalid key, once",
			"- resources: [Backup]\n  targets: [volumeSnapshots]\n  labels: {\"my team\": x}\n",
			`invalid [0].labels["my team"]`),
		Entry("a key the operator sets itself, once",
			"- targets: [all]\n  labels: {cnpg.io/cluster: x}\n",
			`ignored [0].labels["cnpg.io/cluster"]`),
		Entry("the problems of each rule together, whatever the resource finding them",
			"- resources: [Backup]\n  targets: [services]\n  labels: {a: b}\n"+
				"- targets: [nodes]\n  labels: {a: b}\n",
			"invalid [0].targets[0]", "invalid [1].targets[0]"),
		Entry("every problem, by rule",
			"- resources: [Database]\n  targets: [nodes]\n  labels: {cnpg.io/cluster: x}\n"+
				"- resources: [Backup]\n  targets: [services]\n",
			"invalid [0].resources[0]", "invalid [0].targets[0]", `ignored [0].labels["cnpg.io/cluster"]`,
			"invalid [1].targets[0]"),
	)

	It("names the resource and its targets in the message", func() {
		_, found := CompileOperatorRules(operatorRules(
			"- resources: [Pooler]\n  targets: [pgPods]\n  labels: {a: b}\n" +
				"- resources: [Database]\n  targets: [all]\n"))
		Expect(found).To(HaveLen(2))
		Expect(found[0].String()).To(Equal(`[0].targets[0]: target "pgPods" not available on a Pooler, ` +
			"expected one of [all allPods poolerPods poolerDeployments services secrets]"))
		Expect(found[1].String()).To(Equal(`[1].resources[0]: unknown resource "Database", ` +
			"expected one of [Cluster Pooler ScheduledBackup Backup]"))
	})

	Context("conflicts", func() {
		It("does not take two values on the same target for different resources as a conflict", func() {
			_, found := CompileOperatorRules(operatorRules(
				"- resources: [Cluster]\n  targets: [services, all]\n  labels: {a: \"1\"}\n" +
					"- resources: [Pooler]\n  targets: [services, all]\n  labels: {a: \"2\"}\n" +
					"- resources: [Backup]\n  targets: [all]\n  labels: {a: \"3\"}\n"))
			Expect(found).To(BeEmpty())
		})

		It("refuses two values on the same target for a resource both rules name, once", func() {
			_, found := CompileOperatorRules(operatorRules(
				"- resources: [Cluster, Pooler]\n  targets: [services]\n  labels: {a: \"1\"}\n" +
					"- resources: [Pooler, Cluster]\n  targets: [services]\n  labels: {a: \"2\"}\n"))
			Expect(problems(found)).To(Equal([]string{`invalid [1].labels["a"]`}))
		})

		It("refuses a rule naming no resource giving another value than a rule naming one", func() {
			_, found := CompileOperatorRules(operatorRules(
				"- resources: [Pooler]\n  targets: [secrets]\n  annotations: {a: \"1\"}\n" +
					"- targets: [secrets]\n  annotations: {a: \"2\"}\n"))
			Expect(problems(found)).To(Equal([]string{`invalid [1].annotations["a"]`}))
			Expect(found[0].Message).To(Equal(`"2" on secrets: also set to "1" by rule [0]`))
		})

		It("refuses a conflict on a covering target only where both rules reach it", func() {
			_, found := CompileOperatorRules(operatorRules(
				"- targets: [allPods]\n  labels: {a: \"1\"}\n" +
					"- resources: [Pooler]\n  targets: [allPods]\n  labels: {a: \"2\"}\n"))
			Expect(problems(found)).To(Equal([]string{`invalid [1].labels["a"]`}))
		})
	})

	It("keeps the valid part of the rules with problems", func() {
		sets, found := CompileOperatorRules(operatorRules(
			"- resources: [Pooler]\n  targets: [pgPods, services]\n  labels: {a: b}\n"))
		Expect(found).To(HaveLen(1))
		Expect(sets.For(PoolerResource).Labels(Services)).To(Equal(map[string]string{"a": "b"}))
		Expect(sets.For(ClusterResource).IsEmpty()).To(BeTrue())
	})

	It("does not keep a reference to the rules", func() {
		rules := operatorRules("- resources: [Cluster]\n  targets: [all]\n  labels: {a: b}\n")
		sets, _ := CompileOperatorRules(rules)
		rules[0].Labels["a"] = "changed"
		rules[0].Resources[0] = PoolerResource
		Expect(sets.For(ClusterResource).Labels(All)).To(Equal(map[string]string{"a": "b"}))
		Expect(sets.For(PoolerResource).IsEmpty()).To(BeTrue())
	})
})
