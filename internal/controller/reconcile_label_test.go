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
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/internal/configuration"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// expectEveryObjectReconcileLabelled lists the given kinds in a namespace and
// fails on any object without the cache label, naming it.
func expectEveryObjectReconcileLabelled(
	ctx context.Context, c client.Client, namespace string, lists ...client.ObjectList,
) {
	found := 0
	for _, list := range lists {
		Expect(c.List(ctx, list, client.InNamespace(namespace))).To(Succeed())
		Expect(meta.EachListItem(list, func(obj runtime.Object) error {
			object := obj.(client.Object)
			found++
			if object.GetLabels()[utils.ReconcileLabelName] != "true" {
				return fmt.Errorf("%T %s has no %s label", object, object.GetName(), utils.ReconcileLabelName)
			}
			return nil
		})).To(Succeed())
	}
	// The scenario is only meaningful if every listed kind got at least one object.
	Expect(found).To(BeNumerically(">=", len(lists)))
}

var _ = Describe("the cache label on the objects the operator creates", func() {
	var env *testingEnvironment

	BeforeEach(func() {
		env = buildTestEnvironment()
		configuration.Current = configuration.NewConfiguration()
	})

	AfterEach(func() {
		configuration.Current = configuration.NewConfiguration()
	})

	It("is on every object the Cluster reconciler creates, certificates included", func(ctx SpecContext) {
		namespace := newFakeNamespace(env.client)
		cluster := newFakeCNPGCluster(env.client, namespace, func(cluster *apiv1.Cluster) {
			// Let the operator generate the PKI instead of pointing at user Secrets
			cluster.Spec.Certificates = nil
			cluster.Status.Certificates = apiv1.CertificatesStatus{}
		})

		Expect(env.clusterReconciler.createPostgresClusterObjects(ctx, cluster)).To(Succeed())

		expectEveryObjectReconcileLabelled(ctx, env.client, namespace,
			&corev1.SecretList{},
			&corev1.ServiceList{},
			&corev1.ServiceAccountList{},
			&rbacv1.RoleList{},
			&rbacv1.RoleBindingList{},
			&policyv1.PodDisruptionBudgetList{},
			&coordinationv1.LeaseList{},
		)
	})

	It("is on every object the Pooler reconciler creates", func(ctx SpecContext) {
		namespace := newFakeNamespace(env.client)
		cluster := newFakeCNPGCluster(env.client, namespace)
		pooler := newFakePooler(env.client, cluster)

		Expect(env.poolerReconciler.updateOwnedObjects(ctx, pooler, &poolerManagedResources{Cluster: cluster})).
			To(Succeed())

		expectEveryObjectReconcileLabelled(ctx, env.client, namespace,
			&appsv1.DeploymentList{},
			&corev1.ServiceList{},
			&corev1.ServiceAccountList{},
			&rbacv1.RoleList{},
			&rbacv1.RoleBindingList{},
		)
	})

	// Objects created by an operator version without the label must get it on
	// the next reconciliation, including the kinds the reconcilers rewrite only
	// when their content changes: Role, RoleBinding, primary Lease, PKI Secrets,
	// Pooler Deployment and RBAC. That is what makes a later switch to a
	// filtered cache safe: the cache would not see an unlabelled object.
	It("is added to the objects created before the label existed", func(ctx SpecContext) {
		namespace := newFakeNamespace(env.client)
		cluster := newFakeCNPGCluster(env.client, namespace, func(cluster *apiv1.Cluster) {
			cluster.Spec.Certificates = nil
			cluster.Status.Certificates = apiv1.CertificatesStatus{}
		})
		pooler := newFakePooler(env.client, cluster)

		Expect(env.clusterReconciler.createPostgresClusterObjects(ctx, cluster)).To(Succeed())
		Expect(env.poolerReconciler.updateOwnedObjects(ctx, pooler, &poolerManagedResources{Cluster: cluster})).
			To(Succeed())

		lists := []client.ObjectList{
			&corev1.SecretList{},
			&corev1.ServiceList{},
			&corev1.ServiceAccountList{},
			&rbacv1.RoleList{},
			&rbacv1.RoleBindingList{},
			&policyv1.PodDisruptionBudgetList{},
			&coordinationv1.LeaseList{},
			&appsv1.DeploymentList{},
		}
		stripReconcileLabel(ctx, env.client, namespace, lists...)

		Expect(env.clusterReconciler.createPostgresClusterObjects(ctx, cluster)).To(Succeed())
		resources, err := env.poolerReconciler.getManagedResources(ctx, pooler, cluster)
		Expect(err).ToNot(HaveOccurred())
		Expect(env.poolerReconciler.updateOwnedObjects(ctx, pooler, resources)).To(Succeed())

		expectEveryObjectReconcileLabelled(ctx, env.client, namespace, lists...)
		Expect(resources.Deployment.Spec.Template.Labels).To(HaveKeyWithValue(utils.ReconcileLabelName, "true"))
	})
})

// stripReconcileLabel removes the cache label from every object of the given
// kinds in a namespace, as if an older operator had created them.
func stripReconcileLabel(ctx context.Context, c client.Client, namespace string, lists ...client.ObjectList) {
	for _, list := range lists {
		Expect(c.List(ctx, list, client.InNamespace(namespace))).To(Succeed())
		Expect(meta.EachListItem(list, func(obj runtime.Object) error {
			object := obj.(client.Object)
			labels := object.GetLabels()
			delete(labels, utils.ReconcileLabelName)
			object.SetLabels(labels)
			if deployment, ok := object.(*appsv1.Deployment); ok {
				delete(deployment.Spec.Template.Labels, utils.ReconcileLabelName)
			}
			return c.Update(ctx, object)
		})).To(Succeed())
	}
}
