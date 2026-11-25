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

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	k8client "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/internal/configuration"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/specs"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// These paths behave as in the upstream operator
var _ = Describe("parity with the upstream operator", func() {
	var (
		env     *testingEnvironment
		cluster *apiv1.Cluster
	)

	BeforeEach(func() {
		env = buildTestEnvironment()
		cluster = newFakeCNPGCluster(env.client, newFakeNamespace(env.client))
	})

	// A stale cache answers NotFound for a Secret that exists
	It("accepts a pull Secret created meanwhile", func(ctx SpecContext) {
		previous := configuration.Current
		configuration.Current = configuration.NewConfiguration()
		DeferCleanup(func() { configuration.Current = previous })
		configuration.Current.OperatorNamespace = "operators"
		configuration.Current.OperatorPullSecretName = "operator-pull"
		Expect(env.client.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "operator-pull", Namespace: "operators"},
		})).To(Succeed())
		pullSecret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
			Name: cluster.Name + "-pull", Namespace: cluster.Namespace,
		}}
		Expect(env.client.Create(ctx, pullSecret)).To(Succeed())
		env.clusterReconciler.Client = interceptor.NewClient(env.client, interceptor.Funcs{
			Get: func(ctx context.Context, c k8client.WithWatch, key k8client.ObjectKey, obj k8client.Object,
				opts ...k8client.GetOption,
			) error {
				if key.Name == pullSecret.Name {
					return apierrs.NewNotFound(schema.GroupResource{Resource: "secrets"}, key.Name)
				}
				return c.Get(ctx, key, obj, opts...)
			},
		})

		name, err := env.clusterReconciler.copyPullSecretFromOperator(ctx, cluster)
		Expect(err).ToNot(HaveOccurred())
		Expect(name).To(Equal(pullSecret.Name))
	})

	It("leaves alone a RoleBinding and a CA Secret the Cluster does not own", func(ctx SpecContext) {
		roleBinding := specs.CreateRoleBinding(cluster.ObjectMeta, cluster.GetServiceAccountName())
		Expect(env.client.Create(ctx, &roleBinding)).To(Succeed())
		Expect(env.clusterReconciler.createRoleBinding(ctx, cluster)).To(Succeed())
		var stored rbacv1.RoleBinding
		Expect(env.client.Get(ctx, k8client.ObjectKeyFromObject(&roleBinding), &stored)).To(Succeed())
		Expect(stored.ResourceVersion).To(Equal(roleBinding.ResourceVersion))

		secretName := cluster.Name + "-unowned-ca"
		secret, err := env.clusterReconciler.ensureCASecret(ctx, cluster, secretName)
		Expect(err).ToNot(HaveOccurred())
		secret.OwnerReferences = nil
		delete(secret.Labels, utils.ReconcileLabelName)
		Expect(env.client.Update(ctx, secret)).To(Succeed())
		again, err := env.clusterReconciler.ensureCASecret(ctx, cluster, secretName)
		Expect(err).ToNot(HaveOccurred())
		Expect(again.ResourceVersion).To(Equal(secret.ResourceVersion))
		Expect(again.Labels).ToNot(HaveKey(utils.ReconcileLabelName))
	})

	It("keeps blockOwnerDeletion on an object whose metadata is inherited", func() {
		job := &metav1.ObjectMeta{Name: "join", Namespace: cluster.Namespace}
		holder := &corev1.ConfigMap{ObjectMeta: *job}
		Expect(ctrl.SetControllerReference(cluster, holder, env.clusterReconciler.Scheme)).To(Succeed())
		cluster.SetInheritedData(&holder.ObjectMeta)
		Expect(holder.OwnerReferences).To(HaveLen(1))
		Expect(holder.OwnerReferences[0].BlockOwnerDeletion).To(Equal(ptr.To(true)))
		Expect(holder.Labels).To(HaveKey(utils.ReconcileLabelName))
	})
})
