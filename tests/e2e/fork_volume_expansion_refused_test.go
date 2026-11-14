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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/tests"
	"github.com/cloudnative-pg/cloudnative-pg/tests/config"
	clusterasserts "github.com/cloudnative-pg/cloudnative-pg/tests/internal/asserts/cluster"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/clusterutils"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/run"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/storage"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/yaml"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// skipUnlessCSIStorageClassExpands skips the spec when the e2e environment
// has no StorageClass allowing volume expansion
func skipUnlessCSIStorageClassExpands() {
	GinkgoHelper()

	csiStorageClass := config.Current().Storage.CSIStorageClass
	if csiStorageClass == "" {
		Skip("The e2e configuration names no CSI StorageClass")
	}
	allowExpansion, err := storage.GetStorageAllowExpansion(env.Ctx, env.Client, csiStorageClass)
	Expect(err).ToNot(HaveOccurred())
	if allowExpansion == nil || !*allowExpansion {
		Skip(fmt.Sprintf("StorageClass %v does not allow volume expansion", csiStorageClass))
	}
}

// A Cluster whose volume expansion is refused keeps being reconciled, with
// the `VolumeExpansionRefused` condition, see "When the expansion is refused"
// in docs/src/storage.md.
var _ = Describe("Volume expansion refused", Label(tests.LabelFork, tests.LabelStorage), func() {
	const (
		clusterFile     = forkFixturesDir + "/cluster-expansion-refused.yaml.template"
		namespacePrefix = "fork-expansion-refused"
		quotaName       = "storage-is-full"
		level           = tests.Medium
	)

	BeforeEach(func() {
		if testLevelEnv.Depth < int(level) {
			Skip("Test depth is lower than the amount requested for this test")
		}
		skipUnlessCSIStorageClassExpands()
	})

	It("reports the refusal of a ResourceQuota, keeps reconciling, and expands once the quota is raised", func() {
		namespace, err := env.CreateUniqueTestNamespace(env.Ctx, env.Client, namespacePrefix)
		Expect(err).ToNot(HaveOccurred())
		clusterName, err := yaml.GetResourceNameFromYAML(env.Scheme, clusterFile)
		Expect(err).ToNot(HaveOccurred())
		pvcName := clusterName + "-1"

		clusterasserts.AssertCreateCluster(env, testTimeouts, namespace, clusterName, clusterFile)

		quota := &corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: quotaName},
			Spec: corev1.ResourceQuotaSpec{
				Hard: corev1.ResourceList{
					corev1.ResourceRequestsStorage: resource.MustParse("1Gi"),
				},
			},
		}

		By("filling the requests.storage quota of the namespace with the existing volume", func() {
			Expect(env.Client.Create(env.Ctx, quota)).To(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(env.Client.Get(env.Ctx, ctrlclient.ObjectKeyFromObject(quota), quota)).To(Succeed())
				used, found := quota.Status.Used[corev1.ResourceRequestsStorage]
				g.Expect(found).To(BeTrue())
				g.Expect(used.String()).To(Equal("1Gi"))
			}, 60).Should(Succeed())
		})

		By("asking for a larger volume", func() {
			Eventually(func() error {
				_, _, err := run.Unchecked(fmt.Sprintf(
					`kubectl patch cluster %v -n %v --type=merge -p '{"spec":{"storage":{"size":"2Gi"}}}'`,
					clusterName, namespace))
				return err
			}, 60, 5).Should(Succeed())
		})

		By("having the VolumeExpansionRefused condition tell the quota is exceeded", func() {
			Eventually(func(g Gomega) {
				condition, err := volumeExpansionRefusedCondition(namespace, clusterName)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(condition).ToNot(BeNil())
				g.Expect(condition.Status).To(Equal(metav1.ConditionTrue))
				g.Expect(condition.Reason).To(Equal("QuotaExceeded"))
				// "a message naming the refused PVCs, the size requested, the
				// answer of the API server and the ways out"
				g.Expect(condition.Message).To(ContainSubstring(pvcName + " to 2Gi"))
				g.Expect(condition.Message).To(ContainSubstring("exceeded quota: " + quotaName))
				g.Expect(condition.Message).To(ContainSubstring("Raise the requests.storage ResourceQuota"))
			}, 120).Should(Succeed())

			Expect(pvcStorageRequest(namespace, pvcName)).To(Equal("1Gi"))
		})

		By("recording a Warning event with the same reason", func() {
			Eventually(func(g Gomega) {
				events, err := eventsOf(namespace, "Cluster", clusterName, "QuotaExceeded")
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(events).ToNot(BeEmpty())
				g.Expect(events[0].Type).To(Equal(corev1.EventTypeWarning))
				g.Expect(events[0].Message).To(ContainSubstring(pvcName))
			}, 60).Should(Succeed())
		})

		By("still applying another change of the Cluster", func() {
			err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
				cluster, err := clusterutils.Get(env.Ctx, env.Client, namespace, clusterName)
				if err != nil {
					return err
				}
				cluster.Spec.InheritedMetadata = &apiv1.EmbeddedObjectMetadata{
					Labels: map[string]string{"fork-e2e/still-reconciled": "yes"},
				}
				return env.Client.Update(env.Ctx, cluster)
			})
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() (bool, error) {
				return clusterutils.AllPodsHaveLabels(env.Ctx, env.Client, namespace, clusterName,
					map[string]string{"fork-e2e/still-reconciled": "yes"})
			}, 120).Should(BeTrue())

			// the refusal is still reported, and the Cluster is not broken by it
			condition, err := volumeExpansionRefusedCondition(namespace, clusterName)
			Expect(err).ToNot(HaveOccurred())
			Expect(condition).ToNot(BeNil())
			Expect(condition.Status).To(Equal(metav1.ConditionTrue))
			cluster, err := clusterutils.Get(env.Ctx, env.Client, namespace, clusterName)
			Expect(err).ToNot(HaveOccurred())
			Expect(cluster.Status.Phase).To(Equal(apiv1.PhaseHealthy))
		})

		By("raising the quota", func() {
			err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
				if err := env.Client.Get(env.Ctx, ctrlclient.ObjectKeyFromObject(quota), quota); err != nil {
					return err
				}
				quota.Spec.Hard[corev1.ResourceRequestsStorage] = resource.MustParse("10Gi")
				return env.Client.Update(env.Ctx, quota)
			})
			Expect(err).ToNot(HaveOccurred())
		})

		By("expanding the volume without any change to the Cluster", func() {
			// "The operator retries the expansion every minute"
			Eventually(func(g Gomega) {
				g.Expect(pvcStorageRequest(namespace, pvcName)).To(Equal("2Gi"))

				condition, err := volumeExpansionRefusedCondition(namespace, clusterName)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(condition).ToNot(BeNil())
				g.Expect(condition.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(condition.Reason).To(Equal("VolumeExpansionAccepted"))
			}, 180).Should(Succeed())

			Eventually(func(g Gomega) {
				events, err := eventsOf(namespace, "Cluster", clusterName, "VolumeExpansionAccepted")
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(events).ToNot(BeEmpty())
				g.Expect(events[0].Type).To(Equal(corev1.EventTypeNormal))
			}, 60).Should(Succeed())
		})
	})
})
