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
	"github.com/cloudnative-pg/cloudnative-pg/pkg/metadatarules"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The default and mandatory metadata of the operator are YAML lists of the
// rules the resources use. An invalid setting stops the operator, so that a
// policy is never silently half applied
var _ = Describe("DEFAULT_METADATA and MANDATORY_METADATA", func() {
	parsed := func(defaults, mandatory string) *Data {
		config := NewConfiguration()
		config.DefaultMetadata = defaults
		config.MandatoryMetadata = mandatory
		Expect(config.ParseDefaultAndMandatoryMetadata()).To(Succeed())
		return config
	}

	It("are empty by default, and give nothing", func() {
		config := parsed("", "")
		layers := config.MetadataLayers(metadatarules.ClusterResource, metadatarules.Set{})
		Expect(layers.Defaults.IsEmpty()).To(BeTrue())
		Expect(layers.Mandatory.IsEmpty()).To(BeTrue())
		Expect(layers.Labels(metadatarules.PgPods, map[string]string{"team": "payments"})).To(
			Equal(map[string]string{"team": "payments"}))
	})

	It("accept an empty list", func() {
		layers := parsed("[]", "[]").MetadataLayers(metadatarules.ClusterResource, metadatarules.Set{})
		Expect(layers.Reaches(metadatarules.All)).To(BeFalse())
	})

	It("are read from the operator ConfigMap, as YAML on several lines", func() {
		config := NewConfiguration()
		config.ReadConfigMap(map[string]string{
			"MANDATORY_METADATA": "- targets: [all]\n  labels:\n    cost-centre: dbaas\n",
			"DEFAULT_METADATA": `
- targets: [allPods]
  labels:
    team: unassigned
- targets: [secrets]
  annotations:
    replicator.v1.mittwald.de/replicate-to: "apps,batch"
`,
		})
		Expect(config.ParseDefaultAndMandatoryMetadata()).To(Succeed())
		Expect(config.MetadataLayers(metadatarules.PoolerResource, metadatarules.Set{}).Labels(
			metadatarules.PoolerPods, nil)).To(Equal(map[string]string{"team": "unassigned", "cost-centre": "dbaas"}))
		// A value with a comma, which a comma-separated setting could not hold
		Expect(config.MetadataLayers(metadatarules.ClusterResource, metadatarules.Set{}).Annotations(
			metadatarules.Secrets, nil)).To(Equal(map[string]string{
			"replicator.v1.mittwald.de/replicate-to": "apps,batch",
		}))
	})

	It("accept every target, for every kind of resource", func() {
		config := parsed("- targets: [all, allPods, pgPods, majorUpgradePods, poolerPods, poolerDeployments, "+
			"services, persistentVolumeClaims, secrets, podDisruptionBudgets, backups, volumeSnapshots]\n"+
			"  labels: {team: payments}\n", "")
		for _, resource := range metadatarules.Resources {
			for _, target := range resource.Targets() {
				Expect(config.MetadataLayers(resource, metadatarules.Set{}).Labels(target, nil)).To(
					HaveKeyWithValue("team", "payments"), "%s %s", resource, target)
			}
		}
	})

	It("limit a rule to the objects of the resources it names", func() {
		config := parsed("- targets: [services]\n  labels: {lb-pool: shared}\n",
			"- resources: [Cluster]\n  targets: [services]\n  labels: {lb-pool: databases}\n"+
				"- resources: [Pooler]\n  targets: [services]\n  labels: {lb-pool: poolers}\n"+
				"- resources: [Pooler]\n  targets: [all]\n  labels: {component: pooler}\n")
		cluster := config.MetadataLayers(metadatarules.ClusterResource, metadatarules.Set{})
		pooler := config.MetadataLayers(metadatarules.PoolerResource, metadatarules.Set{})
		Expect(cluster.Labels(metadatarules.Services, nil)).To(Equal(map[string]string{"lb-pool": "databases"}))
		Expect(pooler.Labels(metadatarules.Services, nil)).To(Equal(map[string]string{
			"lb-pool": "poolers", "component": "pooler",
		}))
		Expect(pooler.Labels(metadatarules.PoolerDeployments, nil)).To(Equal(map[string]string{"component": "pooler"}))
		Expect(cluster.Labels(metadatarules.PgPods, nil)).To(BeEmpty())
		Expect(config.MetadataLayers(metadatarules.BackupResource, metadatarules.Set{}).Reaches(
			metadatarules.VolumeSnapshots)).To(BeFalse())
	})

	It("put the default below the resource and the mandatory value above its rules", func() {
		config := parsed(
			"- targets: [all]\n  labels: {team: unassigned, tier: default}\n",
			"- targets: [pgPods]\n  labels: {cost-centre: dbaas}\n")
		own, problems := metadatarules.Compile([]metadatarules.Rule{{
			Targets: []metadatarules.Target{metadatarules.PgPods},
			Labels:  map[string]string{"tier": "own", "cost-centre": "own"},
		}}, metadatarules.Targets)
		Expect(problems).To(BeEmpty())
		Expect(config.MetadataLayers(metadatarules.ClusterResource, own).Labels(
			metadatarules.PgPods, map[string]string{"team": "payments"})).To(
			Equal(map[string]string{"team": "payments", "tier": "own", "cost-centre": "dbaas"}))
	})

	DescribeTable("stop the operator on an invalid setting, naming it and the rule",
		func(defaults, mandatory string, expected ...string) {
			config := NewConfiguration()
			config.DefaultMetadata = defaults
			config.MandatoryMetadata = mandatory
			err := config.ParseDefaultAndMandatoryMetadata()
			Expect(err).To(HaveOccurred())
			for _, text := range expected {
				Expect(err.Error()).To(ContainSubstring(text))
			}
		},
		Entry("not YAML", "- targets: [all\n", "", "DEFAULT_METADATA: not a YAML list of rules"),
		Entry("not a list", "targets: [all]\n", "", "DEFAULT_METADATA: not a YAML list of rules"),
		Entry("a misspelled field", "- target: [all]\n  labels: {a: b}\n", "",
			"DEFAULT_METADATA: not a YAML list of rules", `unknown field "target"`),
		Entry("a list of strings", "- all\n", "", "DEFAULT_METADATA: not a YAML list of rules"),
		Entry("no target", "", "- labels: {a: b}\n", "MANDATORY_METADATA[0].targets: no target"),
		Entry("an unknown target", "", "- targets: [nodes]\n  labels: {a: b}\n",
			`MANDATORY_METADATA[0].targets[0]: unknown target "nodes"`),
		Entry("an invalid key", "- targets: [all]\n  labels: {\"my team\": x}\n", "",
			`DEFAULT_METADATA[0].labels["my team"]: invalid key`),
		Entry("an invalid label value", "- targets: [all]\n  labels: {team: \"a,b\"}\n", "",
			`DEFAULT_METADATA[0].labels["team"]: invalid label value`),
		Entry("a key given two values for one target",
			"- targets: [secrets]\n  labels: {team: a}\n- targets: [secrets]\n  labels: {team: b}\n", "",
			`DEFAULT_METADATA[1].labels["team"]: "b" on secrets: also set to "a" by rule [0]`),
		Entry("a key the operator sets itself", "", "- targets: [all]\n  labels: {cnpg.io/cluster: x}\n",
			`MANDATORY_METADATA[0].labels["cnpg.io/cluster"]: set by the operator`),
		Entry("an unknown resource", "- resources: [Database]\n  targets: [all]\n  labels: {a: b}\n", "",
			`DEFAULT_METADATA[0].resources[0]: unknown resource "Database"`),
		Entry("a target not reaching the objects of the named resource", "",
			"- resources: [Pooler]\n  targets: [pgPods]\n  labels: {a: b}\n",
			`MANDATORY_METADATA[0].targets[0]: target "pgPods" not available on a Pooler`),
		Entry("a key given two values for one target of a resource both rules name", "",
			"- resources: [Pooler]\n  targets: [services]\n  labels: {a: \"1\"}\n"+
				"- targets: [services]\n  labels: {a: \"2\"}\n",
			`MANDATORY_METADATA[1].labels["a"]: "2" on services: also set to "1" by rule [0]`),
		Entry("both settings at once", "- targets: [nodes]\n", "- labels: {a: b}\n",
			"DEFAULT_METADATA[0].targets[0]", "MANDATORY_METADATA[0].targets"),
	)

	It("keep the previous rules when a new setting is refused", func() {
		config := parsed("- targets: [all]\n  labels: {team: unassigned}\n", "")
		config.DefaultMetadata = "- targets: [nodes]\n"
		Expect(config.ParseDefaultAndMandatoryMetadata()).NotTo(Succeed())
		Expect(config.MetadataLayers(metadatarules.ClusterResource, metadatarules.Set{}).Labels(
			metadatarules.All, nil)).To(
			HaveKeyWithValue("team", "unassigned"))
	})

	It("keep labels and annotations apart, and every key of a rule", func() {
		config := parsed("", "- targets: [services]\n  labels: {a: \"1\"}\n  annotations: {a: \"2\", b: \"3\"}\n")
		layers := config.MetadataLayers(metadatarules.PoolerResource, metadatarules.Set{})
		Expect(layers.Labels(metadatarules.Services, nil)).To(Equal(map[string]string{"a": "1"}))
		Expect(layers.Annotations(metadatarules.Services, nil)).To(Equal(map[string]string{"a": "2", "b": "3"}))
	})
})
