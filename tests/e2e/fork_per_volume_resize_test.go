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
	"time"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/tests"
	clusterasserts "github.com/cloudnative-pg/cloudnative-pg/tests/internal/asserts/cluster"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/clusterutils"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/run"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/yaml"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The WAL volume is resized according to the `resizeInUseVolumes` of its own
// storage section, not to the one of `.spec.storage`, see "On the objects of
// existing clusters" in docs/src/fork_differences.md and "Re-creating
// storage" in docs/src/storage.md.
var _ = Describe("Per-volume resize", Label(tests.LabelFork, tests.LabelStorage), func() {
	const (
		clusterFile     = forkFixturesDir + "/cluster-per-volume-resize.yaml.template"
		namespacePrefix = "fork-per-volume-resize"
		level           = tests.Medium
	)

	BeforeEach(func() {
		if testLevelEnv.Depth < int(level) {
			Skip("Test depth is lower than the amount requested for this test")
		}
		skipUnlessCSIStorageClassExpands()
	})

	It("expands the WAL volume while PGDATA, with resizeInUseVolumes disabled, keeps its size", func() {
		namespace, err := env.CreateUniqueTestNamespace(env.Ctx, env.Client, namespacePrefix)
		Expect(err).ToNot(HaveOccurred())
		clusterName, err := yaml.GetResourceNameFromYAML(env.Scheme, clusterFile)
		Expect(err).ToNot(HaveOccurred())
		dataPVC := clusterName + "-1"
		walPVC := clusterName + "-1-wal"

		clusterasserts.AssertCreateCluster(env, testTimeouts, namespace, clusterName, clusterFile)
		Expect(pvcStorageRequest(namespace, dataPVC)).To(Equal("1Gi"))
		Expect(pvcStorageRequest(namespace, walPVC)).To(Equal("1Gi"))

		By("asking for larger PGDATA and WAL volumes", func() {
			Eventually(func() error {
				_, _, err := run.Unchecked(fmt.Sprintf(
					`kubectl patch cluster %v -n %v --type=merge `+
						`-p '{"spec":{"storage":{"size":"2Gi"},"walStorage":{"size":"2Gi"}}}'`,
					clusterName, namespace))
				return err
			}, 60, 5).Should(Succeed())
		})

		By("expanding the WAL volume", func() {
			Eventually(func() (string, error) {
				return pvcStorageRequest(namespace, walPVC)
			}, 120).Should(Equal("2Gi"))
		})

		By("leaving the PGDATA volume at its size", func() {
			Consistently(func() (string, error) {
				return pvcStorageRequest(namespace, dataPVC)
			}, 30*time.Second, 2*time.Second).Should(Equal("1Gi"))
		})

		By("keeping the Cluster healthy, without a refused expansion", func() {
			cluster, err := clusterutils.Get(env.Ctx, env.Client, namespace, clusterName)
			Expect(err).ToNot(HaveOccurred())
			Expect(cluster.Status.Phase).To(Equal(apiv1.PhaseHealthy))

			condition, err := volumeExpansionRefusedCondition(namespace, clusterName)
			Expect(err).ToNot(HaveOccurred())
			Expect(condition).To(BeNil())
		})
	})
})
