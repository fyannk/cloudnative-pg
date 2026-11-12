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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// The default monitoring queries are copied into each Cluster namespace. A
// copy made by an operator version without the label is only patched when its
// data changes, so the label has to be added on its own, or a filtered cache
// never holds the copy and every reconciliation reads it from the API server.
var _ = Describe("the cache label on the default monitoring query copies", func() {
	var (
		env                   *testingEnvironment
		operatorNamespace     string
		namespace             string
		cluster               *apiv1.Cluster
		originalConfiguration configuration.Data
	)

	const (
		sourceConfigMap = "monitoring-source"
		sourceSecret    = "monitoring-source"
	)

	BeforeEach(func(ctx SpecContext) {
		env = buildTestEnvironment()
		originalConfiguration = *configuration.Current
		operatorNamespace = newFakeNamespace(env.client)
		namespace = newFakeNamespace(env.client)
		cluster = newFakeCNPGCluster(env.client, namespace)
		configuration.Current.OperatorNamespace = operatorNamespace
		configuration.Current.MonitoringQueriesConfigmap = sourceConfigMap
		configuration.Current.MonitoringQueriesSecret = sourceSecret

		Expect(env.client.Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: sourceConfigMap, Namespace: operatorNamespace},
			Data:       map[string]string{apiv1.DefaultMonitoringKey: "queries"},
		})).To(Succeed())
		Expect(env.client.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: sourceSecret, Namespace: operatorNamespace},
			Data:       map[string][]byte{apiv1.DefaultMonitoringKey: []byte("queries")},
		})).To(Succeed())
	})

	AfterEach(func() {
		*configuration.Current = originalConfiguration
	})

	// olderCopyMeta is the metadata an operator without the cache label gave
	// its copies, the operator version annotation marking them as its own
	olderCopyMeta := func(name string) metav1.ObjectMeta {
		return metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				utils.WatchedLabelName:                "true",
				utils.KubernetesAppManagedByLabelName: utils.ManagerName,
			},
			Annotations: map[string]string{utils.OperatorVersionAnnotationName: "1.29.0"},
		}
	}

	It("is added to unchanged copies made before the label existed", func(ctx SpecContext) {
		Expect(env.client.Create(ctx, &corev1.ConfigMap{
			ObjectMeta: olderCopyMeta(apiv1.DefaultMonitoringConfigMapName),
			Data:       map[string]string{apiv1.DefaultMonitoringKey: "queries"},
		})).To(Succeed())
		Expect(env.client.Create(ctx, &corev1.Secret{
			ObjectMeta: olderCopyMeta(apiv1.DefaultMonitoringSecretName),
			Data:       map[string][]byte{apiv1.DefaultMonitoringKey: []byte("queries")},
		})).To(Succeed())

		Expect(env.clusterReconciler.createOrPatchDefaultMetrics(ctx, cluster)).To(Succeed())

		var configMap corev1.ConfigMap
		Expect(env.client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: apiv1.DefaultMonitoringConfigMapName},
			&configMap)).To(Succeed())
		Expect(configMap.Labels).To(HaveKeyWithValue(utils.ReconcileLabelName, "true"))
		Expect(configMap.Labels).To(HaveKeyWithValue(utils.WatchedLabelName, "true"))

		var secret corev1.Secret
		Expect(env.client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: apiv1.DefaultMonitoringSecretName},
			&secret)).To(Succeed())
		Expect(secret.Labels).To(HaveKeyWithValue(utils.ReconcileLabelName, "true"))
		Expect(secret.Labels).To(HaveKeyWithValue(utils.WatchedLabelName, "true"))
	})

	It("is not added to an object of the same name the operator did not create", func(ctx SpecContext) {
		foreignMeta := metav1.ObjectMeta{Name: apiv1.DefaultMonitoringConfigMapName, Namespace: namespace}
		Expect(env.client.Create(ctx, &corev1.ConfigMap{
			ObjectMeta: foreignMeta,
			Data:       map[string]string{apiv1.DefaultMonitoringKey: "foreign queries"},
		})).To(Succeed())

		Expect(env.clusterReconciler.createOrPatchDefaultMetricsConfigmap(ctx, cluster)).To(Succeed())

		var configMap corev1.ConfigMap
		Expect(env.client.Get(ctx, client.ObjectKeyFromObject(&corev1.ConfigMap{ObjectMeta: foreignMeta}),
			&configMap)).To(Succeed())
		Expect(configMap.Labels).ToNot(HaveKey(utils.ReconcileLabelName))
		Expect(configMap.Data).To(HaveKeyWithValue(apiv1.DefaultMonitoringKey, "foreign queries"))
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
