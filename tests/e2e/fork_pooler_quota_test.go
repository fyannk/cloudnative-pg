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
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/tests"
	clusterasserts "github.com/cloudnative-pg/cloudnative-pg/tests/internal/asserts/cluster"
	pgbouncerasserts "github.com/cloudnative-pg/cloudnative-pg/tests/internal/asserts/pgbouncer"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/yaml"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// A Pooler whose Deployment cannot run its Pods goes to the `inactive` phase,
// with the message of the Deployment as the reason, see "In the behavior of
// the operator" in docs/src/fork_differences.md.
var _ = Describe("Pooler blocked by a ResourceQuota", Label(tests.LabelFork, tests.LabelServiceConnectivity), func() {
	const (
		clusterFile     = forkFixturesDir + "/cluster-single.yaml.template"
		poolerFile      = forkFixturesDir + "/pooler.yaml"
		namespacePrefix = "fork-pooler-quota"
		quotaName       = "no-more-pods"
		level           = tests.Medium
	)

	BeforeEach(func() {
		if testLevelEnv.Depth < int(level) {
			Skip("Test depth is lower than the amount requested for this test")
		}
	})

	It("reports the Pooler as inactive without flapping, and as active once the quota is lifted", func() {
		namespace, err := env.CreateUniqueTestNamespace(env.Ctx, env.Client, namespacePrefix)
		Expect(err).ToNot(HaveOccurred())

		clusterName, err := yaml.GetResourceNameFromYAML(env.Scheme, clusterFile)
		Expect(err).ToNot(HaveOccurred())
		poolerName, err := yaml.GetResourceNameFromYAML(env.Scheme, poolerFile)
		Expect(err).ToNot(HaveOccurred())
		poolerKey := ctrlclient.ObjectKey{Namespace: namespace, Name: poolerName}

		getPooler := func() (*apiv1.Pooler, error) {
			pooler := &apiv1.Pooler{}
			err := env.Client.Get(env.Ctx, poolerKey, pooler)
			return pooler, err
		}

		clusterasserts.AssertCreateCluster(env, testTimeouts, namespace, clusterName, clusterFile)
		pgbouncerasserts.AssertPgBouncerPoolerIsSetUp(env, namespace, poolerFile, 1)

		By("having an active Pooler", func() {
			Eventually(func(g Gomega) {
				pooler, err := getPooler()
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(pooler.Status.Phase).To(Equal(apiv1.PoolerPhaseActive))
				g.Expect(pooler.Status.PhaseReason).To(BeEmpty())
			}, 60).Should(Succeed())
		})

		By("forbidding any other Pod in the namespace with a ResourceQuota", func() {
			podList := &corev1.PodList{}
			Expect(env.Client.List(env.Ctx, podList, ctrlclient.InNamespace(namespace))).To(Succeed())
			running := 0
			for _, pod := range podList.Items {
				if pod.Status.Phase != corev1.PodSucceeded && pod.Status.Phase != corev1.PodFailed {
					running++
				}
			}

			quota := &corev1.ResourceQuota{
				ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: quotaName},
				Spec: corev1.ResourceQuotaSpec{
					Hard: corev1.ResourceList{
						corev1.ResourcePods: *resource.NewQuantity(int64(running), resource.DecimalSI),
					},
				},
			}
			Expect(env.Client.Create(env.Ctx, quota)).To(Succeed())

			// the quota only refuses Pods once its usage has been computed
			Eventually(func(g Gomega) {
				g.Expect(env.Client.Get(env.Ctx, ctrlclient.ObjectKeyFromObject(quota), quota)).To(Succeed())
				used, found := quota.Status.Used[corev1.ResourcePods]
				g.Expect(found).To(BeTrue())
				g.Expect(used.Value()).To(BeEquivalentTo(running))
			}, 60).Should(Succeed())
		})

		By("asking for a second instance of the Pooler", func() {
			err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
				pooler, err := getPooler()
				if err != nil {
					return err
				}
				pooler.Spec.Instances = new(int32(2))
				return env.Client.Update(env.Ctx, pooler)
			})
			Expect(err).ToNot(HaveOccurred())
		})

		var deploymentMessage string
		By("having the Deployment report the refused Pod", func() {
			Eventually(func(g Gomega) {
				deployment := &appsv1.Deployment{}
				g.Expect(env.Client.Get(env.Ctx, poolerKey, deployment)).To(Succeed())
				deploymentMessage = ""
				for _, condition := range deployment.Status.Conditions {
					if condition.Type == appsv1.DeploymentReplicaFailure && condition.Status == corev1.ConditionTrue {
						deploymentMessage = condition.Message
					}
				}
				g.Expect(deploymentMessage).To(ContainSubstring("exceeded quota"))
			}, 120).Should(Succeed())
		})

		By("having the Pooler inactive, with the message of the Deployment as the reason", func() {
			Eventually(func(g Gomega) {
				pooler, err := getPooler()
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(pooler.Status.Phase).To(Equal(apiv1.PoolerPhaseInactive))
				g.Expect(pooler.Status.PhaseReason).To(ContainSubstring(deploymentMessage))
				g.Expect(pooler.Status.PhaseReason).To(ContainSubstring(quotaName))
			}, 60).Should(Succeed())
		})

		By("checking the blocked Pooler is not written anymore", func() {
			pooler, err := getPooler()
			Expect(err).ToNot(HaveOccurred())
			resourceVersion := pooler.ResourceVersion

			Consistently(func(g Gomega) {
				pooler, err := getPooler()
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(pooler.ResourceVersion).To(Equal(resourceVersion))
				g.Expect(pooler.Status.Phase).To(Equal(apiv1.PoolerPhaseInactive))
			}, 45*time.Second, time.Second).Should(Succeed())
		})

		By("lifting the quota", func() {
			quota := &corev1.ResourceQuota{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: quotaName}}
			Expect(env.Client.Delete(env.Ctx, quota)).To(Succeed())
		})

		By("having the Pooler active again with its two instances", func() {
			// the ReplicaSet controller retries the refused Pod with a backoff
			// growing with the time spent under the quota
			Eventually(func(g Gomega) {
				pooler, err := getPooler()
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(pooler.Status.Phase).To(Equal(apiv1.PoolerPhaseActive))
				g.Expect(pooler.Status.PhaseReason).To(BeEmpty())
				g.Expect(pooler.Status.Instances).To(BeEquivalentTo(2))
			}, 600, 2).Should(Succeed())
			pgbouncerasserts.AssertPgBouncerPodsAreReady(env, namespace, poolerFile, 2)
		})
	})
})
