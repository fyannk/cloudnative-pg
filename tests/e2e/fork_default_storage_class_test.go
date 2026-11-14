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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"
	"github.com/cloudnative-pg/cloudnative-pg/tests"
	"github.com/cloudnative-pg/cloudnative-pg/tests/config"
	clusterasserts "github.com/cloudnative-pg/cloudnative-pg/tests/internal/asserts/cluster"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/clusterutils"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/objects"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/timeouts"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// DEFAULT_STORAGE_CLASS is written into the storage sections of the new
// clusters, see "Available options" in docs/src/operator_conf.md.
var _ = Describe("Default StorageClass of the operator", Serial,
	Label(tests.LabelFork, tests.LabelDisruptive, tests.LabelStorage), func() {
		const (
			namespacePrefix     = "fork-default-storage-class"
			existingClusterName = "fork-existing"
			newClusterName      = "fork-new"
			level               = tests.Medium
		)

		BeforeEach(func() {
			if testLevelEnv.Depth < int(level) {
				Skip("Test depth is lower than the amount requested for this test")
			}
		})

		// clusterWithoutStorageClass returns a Cluster naming no StorageClass
		// in its storage sections
		clusterWithoutStorageClass := func(namespace, name string) *apiv1.Cluster {
			return &apiv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
				Spec: apiv1.ClusterSpec{
					Instances:            1,
					StorageConfiguration: apiv1.StorageConfiguration{Size: "1Gi"},
					WalStorage:           &apiv1.StorageConfiguration{Size: "1Gi"},
				},
			}
		}

		It("is written in the specification of a new Cluster, not of an existing one", func() {
			// a class which is not the default one of the Kubernetes cluster,
			// to tell the setting of the operator from it
			operatorStorageClass := config.Current().Storage.CSIStorageClass
			if operatorStorageClass == "" || operatorStorageClass == config.Current().Storage.StorageClass {
				Skip("The e2e configuration names no StorageClass besides the default one")
			}

			namespace, err := env.CreateUniqueTestNamespace(env.Ctx, env.Client, namespacePrefix)
			Expect(err).ToNot(HaveOccurred())

			By("creating a Cluster before the setting exists", func() {
				_, err := objects.Create(env.Ctx, env.Client, clusterWithoutStorageClass(namespace, existingClusterName))
				Expect(err).ToNot(HaveOccurred())
				clusterasserts.AssertClusterIsReady(env, namespace, existingClusterName,
					testTimeouts[timeouts.ClusterIsReady])

				cluster, err := clusterutils.Get(env.Ctx, env.Client, namespace, existingClusterName)
				Expect(err).ToNot(HaveOccurred())
				Expect(cluster.Spec.StorageConfiguration.StorageClass).To(BeNil())
				Expect(cluster.Spec.WalStorage.StorageClass).To(BeNil())
			})

			DeferCleanup(saveOperatorConfiguration())
			setOperatorConfiguration(map[string]string{"DEFAULT_STORAGE_CLASS": operatorStorageClass})

			By("writing the class in every storage section of a new Cluster", func() {
				_, err := objects.Create(env.Ctx, env.Client, clusterWithoutStorageClass(namespace, newClusterName))
				Expect(err).ToNot(HaveOccurred())

				cluster, err := clusterutils.Get(env.Ctx, env.Client, namespace, newClusterName)
				Expect(err).ToNot(HaveOccurred())
				Expect(cluster.Spec.StorageConfiguration.StorageClass).To(HaveValue(Equal(operatorStorageClass)))
				Expect(cluster.Spec.WalStorage.StorageClass).To(HaveValue(Equal(operatorStorageClass)))
			})

			By("creating the volumes of the new Cluster with that class", func() {
				Eventually(func(g Gomega) {
					pvcList := &corev1.PersistentVolumeClaimList{}
					g.Expect(env.Client.List(env.Ctx, pvcList, ctrlclient.InNamespace(namespace),
						ctrlclient.MatchingLabels{utils.ClusterLabelName: newClusterName})).To(Succeed())
					g.Expect(pvcList.Items).To(HaveLen(2))
					for _, pvc := range pvcList.Items {
						g.Expect(pvc.Spec.StorageClassName).To(HaveValue(Equal(operatorStorageClass)), pvc.Name)
					}
				}, 120).Should(Succeed())
				clusterasserts.AssertClusterIsReady(env, namespace, newClusterName,
					testTimeouts[timeouts.ClusterIsReady])
			})

			By("leaving the existing Cluster untouched, even when it is updated", func() {
				err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
					cluster, err := clusterutils.Get(env.Ctx, env.Client, namespace, existingClusterName)
					if err != nil {
						return err
					}
					cluster.Spec.InheritedMetadata = &apiv1.EmbeddedObjectMetadata{
						Labels: map[string]string{"fork-e2e/updated": "yes"},
					}
					return env.Client.Update(env.Ctx, cluster)
				})
				Expect(err).ToNot(HaveOccurred())

				Eventually(func() (bool, error) {
					return clusterutils.AllPodsHaveLabels(env.Ctx, env.Client, namespace, existingClusterName,
						map[string]string{"fork-e2e/updated": "yes"})
				}, 120).Should(BeTrue())

				cluster, err := clusterutils.Get(env.Ctx, env.Client, namespace, existingClusterName)
				Expect(err).ToNot(HaveOccurred())
				Expect(cluster.Spec.StorageConfiguration.StorageClass).To(BeNil())
				Expect(cluster.Spec.WalStorage.StorageClass).To(BeNil())

				pvcList := &corev1.PersistentVolumeClaimList{}
				Expect(env.Client.List(env.Ctx, pvcList, ctrlclient.InNamespace(namespace),
					ctrlclient.MatchingLabels{utils.ClusterLabelName: existingClusterName})).To(Succeed())
				Expect(pvcList.Items).To(HaveLen(2))
				for _, pvc := range pvcList.Items {
					Expect(pvc.Spec.StorageClassName).ToNot(HaveValue(Equal(operatorStorageClass)), pvc.Name)
				}
			})
		})
	})
