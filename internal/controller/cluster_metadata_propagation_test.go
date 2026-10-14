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

package controller

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/internal/configuration"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/postgres"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/reconciler/persistentvolumeclaim"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/specs"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// A Pod created again for an existing instance, to reattach its PVC, gets the
// metadata of the instance Pods, as a new one does
var _ = Describe("The metadata of a reattached instance Pod", func() {
	It("comes from the rules of pgPods, the defaults and the mandatory values", func(ctx SpecContext) {
		previous := configuration.Current
		configuration.Current = configuration.NewConfiguration()
		DeferCleanup(func() { configuration.Current = previous })
		configuration.Current.MandatoryMetadata = `[{"targets": ["allPods"], "labels": {"cost-centre": "dbaas"}}]`
		Expect(configuration.Current.ParseDefaultAndMandatoryMetadata()).To(Succeed())

		env := buildTestEnvironment()
		namespace := newFakeNamespace(env.client)
		cluster := newFakeCNPGCluster(env.client, namespace, func(c *apiv1.Cluster) {
			c.Spec.Instances = 3
			c.Spec.MetadataPropagation = []apiv1.MetadataPropagationRule{
				{Targets: []apiv1.MetadataPropagationTarget{"pgPods"}, Labels: map[string]string{"tier": "db"}},
				{Targets: []apiv1.MetadataPropagationTarget{"services"}, Labels: map[string]string{"tier": "lb"}},
			}
		})
		cluster.Status.ReadyInstances = 1

		primaryPod, err := specs.NewInstance(ctx, *cluster, 1)
		Expect(err).ToNot(HaveOccurred())
		primaryPod.Status = corev1.PodStatus{
			Phase:      corev1.PodRunning,
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
		}
		fencedPod, err := specs.NewInstance(ctx, *cluster, 2)
		Expect(err).ToNot(HaveOccurred())
		fencedPod.Status = corev1.PodStatus{Phase: corev1.PodRunning}

		// Instance 3's Pod is missing: it is reattached to its PVC
		pvcs := newFakePVC(env.client, cluster, 3, persistentvolumeclaim.StatusReady)
		cluster.Status.UnusablePVC = []string{specs.GetInstanceName(cluster.Name, 3)}

		res, err := env.clusterReconciler.ensureInstancesAreCreated(ctx, cluster,
			&managedResources{
				instances: corev1.PodList{Items: []corev1.Pod{*primaryPod, *fencedPod}},
				pvcs:      corev1.PersistentVolumeClaimList{Items: pvcs},
			},
			postgres.PostgresqlStatusList{Items: []postgres.PostgresqlStatus{
				{Pod: primaryPod, IsPodReady: true, IsPrimary: true},
				{Pod: fencedPod, IsPodReady: false, MightBeUnavailable: true, IsFenced: true},
			}})
		Expect(err).ToNot(HaveOccurred())
		Expect(res).To(Equal(reconcile.Result{RequeueAfter: time.Second}))

		var pods corev1.PodList
		Expect(env.client.List(ctx, &pods)).To(Succeed())
		Expect(pods.Items).To(HaveLen(1))
		Expect(pods.Items[0].Labels).To(HaveKeyWithValue("tier", "db"))
		Expect(pods.Items[0].Labels).To(HaveKeyWithValue("cost-centre", "dbaas"))
	})
})
