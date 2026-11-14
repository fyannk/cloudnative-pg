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
	corev1 "k8s.io/api/core/v1"

	"github.com/cloudnative-pg/cloudnative-pg/tests"
	clusterasserts "github.com/cloudnative-pg/cloudnative-pg/tests/internal/asserts/cluster"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/clusterutils"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/timeouts"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/yaml"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The proxy settings of the operator are set on the instances, see "Proxy
// settings" in docs/src/operator_conf.md.
var _ = Describe("Proxy settings propagation", Serial, Ordered,
	Label(tests.LabelFork, tests.LabelDisruptive, tests.LabelOperator), func() {
		const (
			clusterFile         = forkFixturesDir + "/cluster-single.yaml.template"
			overrideClusterFile = forkFixturesDir + "/cluster-proxy-override.yaml.template"
			namespacePrefix     = "fork-proxy"
			level               = tests.Medium

			// Nothing listens at this address. HTTP_PROXY only applies to
			// plain HTTP requests, and NO_PROXY keeps every address of the
			// Kubernetes cluster out of it: the operator and the instance
			// managers never use the proxy.
			operatorHTTPProxy = "http://operator-proxy.fork-e2e.invalid:3128"
			operatorNoProxy   = "localhost,127.0.0.1,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,.svc,.cluster.local"
			clusterHTTPProxy  = "http://cluster-proxy.fork-e2e.invalid:8080"
		)

		var namespace, clusterName, overrideClusterName string

		BeforeEach(func() {
			if testLevelEnv.Depth < int(level) {
				Skip("Test depth is lower than the amount requested for this test")
			}
		})

		// instanceEnv returns the environment of the PostgreSQL container
		// of the only instance of a cluster
		instanceEnv := func(g Gomega, name string) map[string]string {
			podList, err := clusterutils.ListPods(env.Ctx, env.Client, namespace, name)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(podList.Items).To(HaveLen(1))
			g.Expect(podList.Items[0].DeletionTimestamp).To(BeNil())
			return postgresContainerEnv(podList.Items[0])
		}

		BeforeAll(func() {
			if testLevelEnv.Depth < int(level) {
				Skip("Test depth is lower than the amount requested for this test")
			}

			DeferCleanup(saveOperatorConfiguration())

			previousEnv := setOperatorDeploymentEnv([]corev1.EnvVar{
				{Name: "HTTP_PROXY", Value: operatorHTTPProxy},
				{Name: "NO_PROXY", Value: operatorNoProxy},
			})
			DeferCleanup(func() { restoreOperatorDeploymentEnv(previousEnv) })

			var err error
			namespace, err = env.CreateUniqueTestNamespace(env.Ctx, env.Client, namespacePrefix)
			Expect(err).ToNot(HaveOccurred())
			clusterName, err = yaml.GetResourceNameFromYAML(env.Scheme, clusterFile)
			Expect(err).ToNot(HaveOccurred())
			overrideClusterName, err = yaml.GetResourceNameFromYAML(env.Scheme, overrideClusterFile)
			Expect(err).ToNot(HaveOccurred())

			clusterasserts.AssertCreateCluster(env, testTimeouts, namespace, clusterName, clusterFile)
			clusterasserts.AssertCreateCluster(env, testTimeouts, namespace, overrideClusterName, overrideClusterFile)
		})

		It("sets the proxy settings of the operator on the instance Pods, in both spellings", func() {
			Eventually(func(g Gomega) {
				instance := instanceEnv(g, clusterName)
				g.Expect(instance).To(HaveKeyWithValue("HTTP_PROXY", operatorHTTPProxy))
				g.Expect(instance).To(HaveKeyWithValue("http_proxy", operatorHTTPProxy))
				g.Expect(instance).To(HaveKeyWithValue("NO_PROXY", operatorNoProxy))
				g.Expect(instance).To(HaveKeyWithValue("no_proxy", operatorNoProxy))
				// the operator has no HTTPS proxy: "Empty values are not propagated"
				g.Expect(instance).ToNot(HaveKey("HTTPS_PROXY"))
				g.Expect(instance).ToNot(HaveKey("https_proxy"))
			}, 60).Should(Succeed())
		})

		It("leaves a Cluster defining a proxy setting in control of all of them", func() {
			Eventually(func(g Gomega) {
				instance := instanceEnv(g, overrideClusterName)
				g.Expect(instance).To(HaveKeyWithValue("HTTP_PROXY", clusterHTTPProxy))
				for _, name := range []string{"http_proxy", "NO_PROXY", "no_proxy", "HTTPS_PROXY", "https_proxy"} {
					g.Expect(instance).ToNot(HaveKey(name))
				}
			}, 60).Should(Succeed())
		})

		It("removes the proxy settings from the instance Pods with PROPAGATE_PROXY_ENV=false", func() {
			setOperatorConfiguration(map[string]string{"PROPAGATE_PROXY_ENV": "false"})

			// the environment is part of the Pod specification: the instance
			// is rolled out
			Eventually(func(g Gomega) {
				instance := instanceEnv(g, clusterName)
				for _, name := range []string{"HTTP_PROXY", "http_proxy", "NO_PROXY", "no_proxy"} {
					g.Expect(instance).ToNot(HaveKey(name))
				}
			}, testTimeouts[timeouts.ClusterIsReady]).Should(Succeed())
			clusterasserts.AssertClusterIsReady(env, namespace, clusterName, testTimeouts[timeouts.ClusterIsReady])

			By("keeping the settings of the Cluster defining its own", func() {
				Eventually(func(g Gomega) {
					g.Expect(instanceEnv(g, overrideClusterName)).To(HaveKeyWithValue("HTTP_PROXY", clusterHTTPProxy))
				}, 60).Should(Succeed())
				clusterasserts.AssertClusterIsReady(env, namespace, overrideClusterName,
					testTimeouts[timeouts.ClusterIsReady])
			})
		})
	})
