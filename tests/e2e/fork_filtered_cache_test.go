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
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"
	"github.com/cloudnative-pg/cloudnative-pg/tests"
	clusterasserts "github.com/cloudnative-pg/cloudnative-pg/tests/internal/asserts/cluster"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/clusterutils"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/run"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/timeouts"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// FILTERED_CACHE_MODE restricts the operator cache to the objects carrying
// the `cnpg.io/reconcile=true` label, see "Operator cache" in
// docs/src/operator_conf.md.
var _ = Describe("Filtered operator cache", Serial, Ordered,
	Label(tests.LabelFork, tests.LabelDisruptive, tests.LabelOperator), func() {
		const (
			clusterFile     = forkFixturesDir + "/cluster-reload.yaml.template"
			clusterName     = "fork-reload"
			secretName      = "fork-reload-superuser"
			namespacePrefix = "fork-filtered-cache"
			level           = tests.Medium
		)

		var namespace string

		BeforeEach(func() {
			if testLevelEnv.Depth < int(level) {
				Skip("Test depth is lower than the amount requested for this test")
			}
		})

		BeforeAll(func() {
			if testLevelEnv.Depth < int(level) {
				Skip("Test depth is lower than the amount requested for this test")
			}

			// registered first to run last, once the namespace is gone: the
			// operator goes back to its unfiltered cache
			DeferCleanup(saveOperatorConfiguration())

			var err error
			namespace, err = env.CreateUniqueTestNamespace(env.Ctx, env.Client, namespacePrefix)
			Expect(err).ToNot(HaveOccurred())

			// the cluster is created with the default mode, `off`
			clusterasserts.AssertCreateCluster(env, testTimeouts, namespace, clusterName, clusterFile)
		})

		// trackedSuperuserSecretVersion is the resource version of the
		// superuser Secret the operator has seen at its last reconciliation
		trackedSuperuserSecretVersion := func() (string, error) {
			cluster, err := clusterutils.Get(env.Ctx, env.Client, namespace, clusterName)
			if err != nil {
				return "", err
			}
			return cluster.Status.SecretsResourceVersion.SuperuserSecretVersion, nil
		}

		It("labels every object of the cluster with cnpg.io/reconcile=true", func() {
			// what the operator creates for a Cluster, with the least number
			// of objects of each kind this cluster has
			kinds := []struct {
				list    ctrlclient.ObjectList
				atLeast int
			}{
				{&corev1.PodList{}, 2},
				{&corev1.PersistentVolumeClaimList{}, 2},
				// -rw, -ro and -r
				{&corev1.ServiceList{}, 3},
				// application user, CA, server and replication certificates
				{&corev1.SecretList{}, 4},
				{&policyv1.PodDisruptionBudgetList{}, 1},
				{&corev1.ServiceAccountList{}, 1},
				{&rbacv1.RoleList{}, 1},
				{&rbacv1.RoleBindingList{}, 1},
				{&batchv1.JobList{}, 0},
				{&corev1.ConfigMapList{}, 0},
			}

			Eventually(func(g Gomega) {
				for _, kind := range kinds {
					g.Expect(env.Client.List(env.Ctx, kind.list, ctrlclient.InNamespace(namespace))).To(Succeed())
					items, err := meta.ExtractList(kind.list)
					g.Expect(err).ToNot(HaveOccurred())

					owned := 0
					for _, item := range items {
						object, ok := item.(ctrlclient.Object)
						g.Expect(ok).To(BeTrue())
						if _, isOwned := apiv1.IsOwnedByCluster(object); !isOwned {
							continue
						}
						owned++
						g.Expect(object.GetLabels()).To(HaveKeyWithValue(utils.ReconcileLabelName, "true"),
							"%T %s", object, object.GetName())
					}
					g.Expect(owned).To(BeNumerically(">=", kind.atLeast), "%T", kind.list)
				}
			}, 120).Should(Succeed())

			By("running the check the documentation gives before switching to a filtered mode", func() {
				stdout, _, err := run.Run(fmt.Sprintf(
					"kubectl get pods,pvc,jobs -n %v -o name -l 'cnpg.io/cluster,cnpg.io/reconcile!=true'", namespace))
				Expect(err).ToNot(HaveOccurred())
				Expect(strings.TrimSpace(stdout)).To(BeEmpty())
			})

			By("leaving the Secret provided by the user without the label", func() {
				secret := &corev1.Secret{}
				Expect(env.Client.Get(env.Ctx,
					types.NamespacedName{Namespace: namespace, Name: secretName}, secret)).To(Succeed())
				Expect(secret.Labels).ToNot(HaveKey(utils.ReconcileLabelName))
				Expect(secret.Labels).To(HaveKeyWithValue(utils.WatchedLabelName, ""))
			})
		})

		It("keeps the cluster healthy once the reload mode is enabled", func() {
			podList, err := clusterutils.ListPods(env.Ctx, env.Client, namespace, clusterName)
			Expect(err).ToNot(HaveOccurred())
			instances := map[string]types.UID{}
			for _, pod := range podList.Items {
				instances[pod.Name] = pod.UID
			}
			Expect(instances).To(HaveLen(2))

			setOperatorConfiguration(map[string]string{"FILTERED_CACHE_MODE": "reload"})

			clusterasserts.AssertClusterIsReady(env, namespace, clusterName, testTimeouts[timeouts.ClusterIsReady])

			// an operator not seeing an object of the cluster would report it
			// as unrecoverable, create the instances again or fail over
			Consistently(func(g Gomega) {
				cluster, err := clusterutils.Get(env.Ctx, env.Client, namespace, clusterName)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(cluster.Status.Phase).To(Equal(apiv1.PhaseHealthy))
				g.Expect(cluster.Status.CurrentPrimary).To(Equal(clusterName + "-1"))

				podList, err := clusterutils.ListPods(env.Ctx, env.Client, namespace, clusterName)
				g.Expect(err).ToNot(HaveOccurred())
				current := map[string]types.UID{}
				for _, pod := range podList.Items {
					current[pod.Name] = pod.UID
				}
				g.Expect(current).To(Equal(instances))
			}, 45*time.Second, 3*time.Second).Should(Succeed())
		})

		It("performs a switchover with the reload mode", func() {
			clusterasserts.AssertSwitchover(env, testTimeouts, namespace, clusterName)
		})

		It("reconciles the cluster when a referenced Secret labeled cnpg.io/reload changes", func() {
			secretKey := types.NamespacedName{Namespace: namespace, Name: secretName}

			By("having the operator track the current version of the Secret", func() {
				secret := &corev1.Secret{}
				Expect(env.Client.Get(env.Ctx, secretKey, secret)).To(Succeed())
				Eventually(trackedSuperuserSecretVersion, 60).Should(Equal(secret.ResourceVersion))
			})

			var newVersion string
			By("changing the Secret", func() {
				err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
					secret := &corev1.Secret{}
					if err := env.Client.Get(env.Ctx, secretKey, secret); err != nil {
						return err
					}
					secret.Data["password"] = []byte("second-password")
					if err := env.Client.Update(env.Ctx, secret); err != nil {
						return err
					}
					newVersion = secret.ResourceVersion
					return nil
				})
				Expect(err).ToNot(HaveOccurred())
			})

			By("having the operator notice the change at once", func() {
				// The Secret is not in the filtered cache: only the
				// metadata-only watch on the cnpg.io/reload label starts
				// this reconciliation
				Eventually(trackedSuperuserSecretVersion, 30).Should(Equal(newVersion))
			})

			clusterasserts.AssertClusterIsReady(env, namespace, clusterName, testTimeouts[timeouts.ClusterIsReady])
		})
	})
