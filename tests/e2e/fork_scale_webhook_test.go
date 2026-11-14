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

package e2e

import (
	"fmt"

	"github.com/cloudnative-pg/cloudnative-pg/tests"
	clusterasserts "github.com/cloudnative-pg/cloudnative-pg/tests/internal/asserts/cluster"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/clusterutils"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/run"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/timeouts"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/yaml"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The `vclusterscale.cnpg.io` validating webhook applies the rules on the
// number of instances to the writes of the scale subresource, see "Scale up
// and down of replicas" in docs/src/operator_capability_levels.md.
var _ = Describe("Scale subresource webhook", Label(tests.LabelFork, tests.LabelReplication), func() {
	const (
		clusterFile     = forkFixturesDir + "/cluster-scale.yaml.template"
		namespacePrefix = "fork-scale-webhook"
		level           = tests.Medium
	)

	BeforeEach(func() {
		if testLevelEnv.Depth < int(level) {
			Skip("Test depth is lower than the amount requested for this test")
		}
	})

	It("refuses a scale write making the Cluster invalid, and accepts a valid one", func() {
		namespace, err := env.CreateUniqueTestNamespace(env.Ctx, env.Client, namespacePrefix)
		Expect(err).ToNot(HaveOccurred())
		clusterName, err := yaml.GetResourceNameFromYAML(env.Scheme, clusterFile)
		Expect(err).ToNot(HaveOccurred())

		clusterasserts.AssertCreateCluster(env, testTimeouts, namespace, clusterName, clusterFile)

		instances := func() (int, error) {
			cluster, err := clusterutils.Get(env.Ctx, env.Client, namespace, clusterName)
			if err != nil {
				return 0, err
			}
			return cluster.Spec.Instances, nil
		}

		By("refusing to scale down to a single instance, which maxSyncReplicas does not allow", func() {
			_, stderr, err := run.Unchecked(
				fmt.Sprintf("kubectl scale --replicas=1 -n %v cluster/%v", namespace, clusterName))
			Expect(err).To(HaveOccurred())
			Expect(stderr).To(ContainSubstring("spec.maxSyncReplicas"))
			Expect(stderr).To(ContainSubstring("maxSyncReplicas must be lower than the number of instances"))

			Expect(instances()).To(Equal(2))
		})

		By("scaling up to three instances", func() {
			_, _, err := run.Run(
				fmt.Sprintf("kubectl scale --replicas=3 -n %v cluster/%v", namespace, clusterName))
			Expect(err).ToNot(HaveOccurred())

			Expect(instances()).To(Equal(3))
			clusterasserts.AssertClusterIsReady(env, namespace, clusterName, testTimeouts[timeouts.ClusterIsReady])
		})

		By("scaling back to two instances", func() {
			_, _, err := run.Run(
				fmt.Sprintf("kubectl scale --replicas=2 -n %v cluster/%v", namespace, clusterName))
			Expect(err).ToNot(HaveOccurred())

			Expect(instances()).To(Equal(2))
			clusterasserts.AssertClusterIsReady(env, namespace, clusterName, testTimeouts[timeouts.ClusterIsReady])
		})
	})
})
