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
	"crypto/x509"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/internal/configuration"
	schemeBuilder "github.com/cloudnative-pg/cloudnative-pg/internal/scheme"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/certs"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("setupPostgresPKI", func() {
	var cluster *apiv1.Cluster

	BeforeEach(func() {
		cluster = &apiv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pki-test",
				Namespace: "default",
			},
		}
	})

	It("succeeds in the default configuration even when the informer cache is stale "+
		"for the shared CA secret", func(ctx SpecContext) {
		// In the default configuration the server CA and the client CA resolve to the
		// same secret name. Simulate the informer cache lagging behind the API server:
		// every Get for that secret returns NotFound, even right after it was created.
		// With the fix, the already-fetched server CA is reused and no second
		// Get/Create is attempted; without it, the redundant Create fails with
		// AlreadyExists and setupPostgresPKI returns an error.
		Expect(cluster.GetServerCASecretName()).To(Equal(cluster.GetClientCASecretName()))
		caName := cluster.GetServerCASecretName()

		fakeClient := fake.NewClientBuilder().
			WithScheme(schemeBuilder.BuildWithAllKnownScheme()).
			WithInterceptorFuncs(interceptor.Funcs{
				Get: func(ctx context.Context, cl client.WithWatch, key client.ObjectKey,
					obj client.Object, opts ...client.GetOption,
				) error {
					if _, ok := obj.(*corev1.Secret); ok && key.Name == caName {
						return apierrors.NewNotFound(corev1.Resource("secrets"), key.Name)
					}
					return cl.Get(ctx, key, obj, opts...)
				},
			}).
			Build()

		r := &ClusterReconciler{
			Client:   fakeClient,
			Recorder: record.NewFakeRecorder(10),
		}

		Expect(r.setupPostgresPKI(ctx, cluster)).To(Succeed())

		// The CA secret must have been created exactly once, and the full PKI provisioned.
		var secrets corev1.SecretList
		Expect(r.Client.List(ctx, &secrets, client.InNamespace(cluster.Namespace))).To(Succeed())
		names := map[string]int{}
		for i := range secrets.Items {
			names[secrets.Items[i].Name]++
		}
		Expect(names[caName]).To(Equal(1), "the shared CA secret must be created exactly once")
		Expect(names).To(HaveKey(cluster.GetServerTLSSecretName()))
		Expect(names).To(HaveKey(cluster.GetReplicationSecretName()))
	})

	It("provisions client certificates from a separate client CA when the names differ",
		func(ctx SpecContext) {
			// A user-supplied client CA with a different name must still be honored:
			// the replication client certificate has to be signed by it, not by the
			// (generated) server CA.
			cluster.Spec.Certificates = &apiv1.CertificatesConfiguration{
				ClientCASecret: "custom-client-ca",
			}
			Expect(cluster.GetServerCASecretName()).ToNot(Equal(cluster.GetClientCASecretName()))

			caPair, err := certs.CreateRootCA(cluster.Name, cluster.Namespace)
			Expect(err).ToNot(HaveOccurred())
			clientCASecret := caPair.GenerateCASecret(cluster.Namespace, "custom-client-ca")

			fakeClient := fake.NewClientBuilder().
				WithScheme(schemeBuilder.BuildWithAllKnownScheme()).
				WithObjects(clientCASecret).
				Build()

			r := &ClusterReconciler{
				Client:   fakeClient,
				Recorder: record.NewFakeRecorder(10),
			}

			Expect(r.setupPostgresPKI(ctx, cluster)).To(Succeed())

			get := func(name string) *corev1.Secret {
				var secret corev1.Secret
				Expect(r.Client.Get(ctx,
					client.ObjectKey{Namespace: cluster.Namespace, Name: name}, &secret)).To(Succeed())
				return &secret
			}
			serverCA := get(cluster.GetServerCASecretName())
			clientCA := get(cluster.GetClientCASecretName())
			replication := get(cluster.GetReplicationSecretName())

			// validateLeafCertificate appends the CA to opts.Roots, so each call needs
			// its own VerifyOptions to avoid accumulating trusted roots across calls.
			clientAuthOpts := func() *x509.VerifyOptions {
				return &x509.VerifyOptions{KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
			}
			Expect(validateLeafCertificate(clientCA, replication, clientAuthOpts())).To(Succeed(),
				"the replication certificate must be signed by the separate client CA")
			Expect(validateLeafCertificate(serverCA, replication, clientAuthOpts())).ToNot(Succeed(),
				"the replication certificate must not be signed by the server CA")
		})
})

// The CA and certificate Secrets did not take the Cluster's inherited
// metadata. They get the metadata of the Secrets when created, and the ones
// created before get it at the next reconciliation; the Secrets referenced
// in spec.certificates are never changed.
var _ = Describe("metadata of the CA and certificate Secrets", func() {
	var (
		previous *configuration.Data
		cluster  *apiv1.Cluster
		r        *ClusterReconciler
	)

	secret := func(ctx context.Context, name string) *corev1.Secret {
		var result corev1.Secret
		Expect(r.Client.Get(ctx, client.ObjectKey{Namespace: cluster.Namespace, Name: name}, &result)).To(Succeed())
		return &result
	}

	BeforeEach(func() {
		previous = configuration.Current
		configuration.Current = configuration.NewConfiguration()
		DeferCleanup(func() { configuration.Current = previous })

		cluster = &apiv1.Cluster{
			TypeMeta:   metav1.TypeMeta{Kind: apiv1.ClusterKind, APIVersion: apiv1.SchemeGroupVersion.String()},
			ObjectMeta: metav1.ObjectMeta{Name: "pki-test", Namespace: "default"},
			Spec: apiv1.ClusterSpec{
				InheritedMetadata: &apiv1.EmbeddedObjectMetadata{Labels: map[string]string{"team": "payments"}},
				MetadataPropagation: []apiv1.MetadataPropagationRule{
					{
						Targets:     []apiv1.MetadataPropagationTarget{"secrets"},
						Annotations: map[string]string{"reflector.v1.k8s.emberstack.com/reflection-allowed": "true"},
					},
					{
						Targets:     []apiv1.MetadataPropagationTarget{"pgPods"},
						Annotations: map[string]string{"vault.hashicorp.com/agent-inject": "true"},
					},
				},
			},
		}
		r = &ClusterReconciler{
			Client:   fake.NewClientBuilder().WithScheme(schemeBuilder.BuildWithAllKnownScheme()).Build(),
			Recorder: record.NewFakeRecorder(10),
		}
	})

	It("puts the metadata of the Secrets on the Secrets it creates, nothing of the Pods", func(ctx SpecContext) {
		Expect(r.setupPostgresPKI(ctx, cluster)).To(Succeed())
		for _, name := range []string{
			cluster.GetServerCASecretName(), cluster.GetServerTLSSecretName(), cluster.GetReplicationSecretName(),
		} {
			created := secret(ctx, name)
			Expect(created.Labels).To(HaveKeyWithValue("team", "payments"), name)
			Expect(created.Labels).To(HaveKeyWithValue(utils.ClusterLabelName, cluster.Name), name)
			Expect(created.Annotations).To(
				HaveKeyWithValue("reflector.v1.k8s.emberstack.com/reflection-allowed", "true"), name)
			Expect(created.Annotations).NotTo(HaveKey("vault.hashicorp.com/agent-inject"), name)
			Expect(created.OwnerReferences).To(HaveLen(1), name)
		}
	})

	It("adds it to the Secrets created before, then leaves them alone", func(ctx SpecContext) {
		withMetadata := cluster.Spec.MetadataPropagation
		cluster.Spec.InheritedMetadata = nil
		cluster.Spec.MetadataPropagation = nil
		Expect(r.setupPostgresPKI(ctx, cluster)).To(Succeed())
		Expect(secret(ctx, cluster.GetServerTLSSecretName()).Annotations).NotTo(
			HaveKey("reflector.v1.k8s.emberstack.com/reflection-allowed"))

		cluster.Spec.MetadataPropagation = withMetadata
		configuration.Current.MandatoryMetadata = `[{"targets": ["secrets"], "labels": {"cost-centre": "dbaas"}}]`
		Expect(configuration.Current.ParseDefaultAndMandatoryMetadata()).To(Succeed())
		Expect(r.setupPostgresPKI(ctx, cluster)).To(Succeed())
		versions := map[string]string{}
		for _, name := range []string{
			cluster.GetServerCASecretName(), cluster.GetServerTLSSecretName(), cluster.GetReplicationSecretName(),
		} {
			updated := secret(ctx, name)
			Expect(updated.Annotations).To(
				HaveKeyWithValue("reflector.v1.k8s.emberstack.com/reflection-allowed", "true"), name)
			Expect(updated.Labels).To(HaveKeyWithValue("cost-centre", "dbaas"), name)
			versions[name] = updated.ResourceVersion
		}

		// Up to date: no further patch
		Expect(r.setupPostgresPKI(ctx, cluster)).To(Succeed())
		for name, version := range versions {
			Expect(secret(ctx, name).ResourceVersion).To(Equal(version), name)
		}
	})

	It("never changes a CA Secret referenced in spec.certificates", func(ctx SpecContext) {
		cluster.Spec.Certificates = &apiv1.CertificatesConfiguration{ClientCASecret: "custom-client-ca"}
		caPair, err := certs.CreateRootCA(cluster.Name, cluster.Namespace)
		Expect(err).ToNot(HaveOccurred())
		provided := caPair.GenerateCASecret(cluster.Namespace, "custom-client-ca")
		Expect(r.Client.Create(ctx, provided)).To(Succeed())
		before := secret(ctx, "custom-client-ca")

		Expect(r.setupPostgresPKI(ctx, cluster)).To(Succeed())
		after := secret(ctx, "custom-client-ca")
		Expect(after.ResourceVersion).To(Equal(before.ResourceVersion))
		Expect(after.Labels).NotTo(HaveKey("team"))
	})

	It("never changes a Secret owned by another Cluster", func(ctx SpecContext) {
		other := cluster.DeepCopy()
		other.Name = "other"
		other.Spec.MetadataPropagation = nil
		other.Spec.InheritedMetadata = nil
		Expect(r.setupPostgresPKI(ctx, other)).To(Succeed())
		var server corev1.Secret
		Expect(r.Client.Get(ctx, client.ObjectKey{Namespace: other.Namespace, Name: other.GetServerTLSSecretName()},
			&server)).To(Succeed())

		Expect(r.ensureSecretMetadata(ctx, cluster, &server)).To(Succeed())
		var after corev1.Secret
		Expect(r.Client.Get(ctx, client.ObjectKeyFromObject(&server), &after)).To(Succeed())
		Expect(after.ResourceVersion).To(Equal(server.ResourceVersion))
	})
})
