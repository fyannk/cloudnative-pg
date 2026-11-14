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
	"regexp"

	corev1 "k8s.io/api/core/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"
	"github.com/cloudnative-pg/cloudnative-pg/tests"
	clusterasserts "github.com/cloudnative-pg/cloudnative-pg/tests/internal/asserts/cluster"
	pgbouncerasserts "github.com/cloudnative-pg/cloudnative-pg/tests/internal/asserts/pgbouncer"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/clusterutils"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/operator"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/proxy"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/yaml"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The metrics this fork adds: `cnpg_pgbouncer_up` on the poolers,
// `cnpg_disk_*` on the instances, `cnpg_operator_*` on the operator. See
// docs/src/monitoring.md and docs/src/connection_pooling.md.
var _ = Describe("Fork metrics", Label(tests.LabelFork, tests.LabelObservability), func() {
	const (
		clusterFile     = forkFixturesDir + "/cluster-single.yaml.template"
		poolerFile      = forkFixturesDir + "/pooler.yaml"
		namespacePrefix = "fork-metrics"
		level           = tests.Medium

		// the port the operator serves its metrics on
		operatorMetricsPort = "8080"
	)

	BeforeEach(func() {
		if testLevelEnv.Depth < int(level) {
			Skip("Test depth is lower than the amount requested for this test")
		}
	})

	It("exposes the PgBouncer, volume and operator metrics", func() {
		namespace, err := env.CreateUniqueTestNamespace(env.Ctx, env.Client, namespacePrefix)
		Expect(err).ToNot(HaveOccurred())
		clusterName, err := yaml.GetResourceNameFromYAML(env.Scheme, clusterFile)
		Expect(err).ToNot(HaveOccurred())
		poolerName, err := yaml.GetResourceNameFromYAML(env.Scheme, poolerFile)
		Expect(err).ToNot(HaveOccurred())

		clusterasserts.AssertCreateCluster(env, testTimeouts, namespace, clusterName, clusterFile)
		pgbouncerasserts.AssertPgBouncerPoolerIsSetUp(env, namespace, poolerFile, 1)

		By("reporting cnpg_pgbouncer_up on the Pooler Pod", func() {
			podList := &corev1.PodList{}
			Expect(env.Client.List(env.Ctx, podList, ctrlclient.InNamespace(namespace),
				ctrlclient.MatchingLabels{utils.PgbouncerNameLabel: poolerName})).To(Succeed())
			Expect(podList.Items).To(HaveLen(1))

			Eventually(func(g Gomega) {
				out, err := proxy.RetrieveMetricsFromPgBouncer(env.Ctx, env.Interface, podList.Items[0], false)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(metricLines(out, "cnpg_pgbouncer_up")).To(ConsistOf("cnpg_pgbouncer_up 1"))
			}, 60).Should(Succeed())
		})

		By("reporting the cnpg_disk_* metrics of the PGDATA volume on the instance Pod", func() {
			primary, err := clusterutils.GetPrimary(env.Ctx, env.Client, namespace, clusterName)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func(g Gomega) {
				out, err := proxy.RetrieveMetricsFromInstance(env.Ctx, env.Interface, *primary, false)
				g.Expect(err).ToNot(HaveOccurred())
				for _, name := range []string{
					"cnpg_disk_total_bytes",
					"cnpg_disk_used_bytes",
					"cnpg_disk_available_bytes",
				} {
					positive := regexp.MustCompile(
						`(?m)^` + name + `\{volume="pgdata"\} [1-9][0-9.e+]*$`)
					g.Expect(positive.MatchString(out)).To(BeTrue(),
						"%s of the pgdata volume is missing or zero:\n%v", name, metricLines(out, "cnpg_disk_"))
				}
				// the cluster has no WAL volume
				g.Expect(out).ToNot(ContainSubstring(`cnpg_disk_total_bytes{volume="wal"}`))
			}, 60).Should(Succeed())
		})

		By("reporting the build and the managed objects on the operator", func() {
			operatorPod, err := operator.GetPod(env.Ctx, env.Client)
			Expect(err).ToNot(HaveOccurred())

			buildInfo := regexp.MustCompile(
				`(?m)^cnpg_operator_build_info\{commit="[^"]*",date="[^"]*",version="[^"]+"\} 1$`)
			// "The `cluster` label is empty for `Cluster` itself"
			managedCluster := regexp.MustCompile(fmt.Sprintf(
				`(?m)^cnpg_operator_managed_objects\{cluster="",kind="Cluster",namespace="%s"\} 1$`, namespace))
			// a Pooler is counted per Cluster
			managedPooler := regexp.MustCompile(fmt.Sprintf(
				`(?m)^cnpg_operator_managed_objects\{cluster="%s",kind="Pooler",namespace="%s"\} 1$`,
				clusterName, namespace))

			Eventually(func(g Gomega) {
				raw, err := env.Interface.CoreV1().Pods(operatorPod.Namespace).ProxyGet(
					"http", operatorPod.Name, operatorMetricsPort, "metrics", nil).DoRaw(env.Ctx)
				g.Expect(err).ToNot(HaveOccurred())
				out := string(raw)

				g.Expect(buildInfo.MatchString(out)).To(BeTrue(),
					"no build info:\n%v", metricLines(out, "cnpg_operator_build_info"))
				g.Expect(managedCluster.MatchString(out)).To(BeTrue(),
					"the Cluster is not counted:\n%v", metricLines(out, "cnpg_operator_managed_objects"))
				g.Expect(managedPooler.MatchString(out)).To(BeTrue(),
					"the Pooler is not counted:\n%v", metricLines(out, "cnpg_operator_managed_objects"))
			}, 60).Should(Succeed())
		})
	})
})
