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

package certs

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	controllerScheme "github.com/cloudnative-pg/cloudnative-pg/internal/scheme"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const webhookCertDir = DefaultWebhookCertDir

// operatorDeploymentWithMount is a deployment shaped like the operator's:
// labelled pods, a manager container with the webhook port, and the
// webhook Secret mounted at the certificate directory
func operatorDeploymentWithMount(secretName string) *appsv1.Deployment {
	name := "cnpg-toto"
	podLabels := map[string]string{"app.kubernetes.io/name": name}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: operatorNamespaceName, Labels: podLabels},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: podLabels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: podLabels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "manager",
						Ports: []corev1.ContainerPort{{Name: "webhook-server", ContainerPort: 9443}},
						VolumeMounts: []corev1.VolumeMount{
							{Name: "scratch-data", MountPath: "/controller"},
							{Name: "webhook-certificates", MountPath: webhookCertDir},
						},
					}},
					Volumes: []corev1.Volume{
						{Name: "scratch-data", VolumeSource: corev1.VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						}},
						{Name: "webhook-certificates", VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{SecretName: secretName, Optional: boolPtr(true)},
						}},
					},
				},
			},
		},
	}
}

func boolPtr(b bool) *bool { return &b }

var _ = Describe("Webhook Secret name discovery", func() {
	It("is the Secret mounted at the certificate directory", func() {
		deployment := operatorDeploymentWithMount("toto-webhook-tls")
		Expect(MountedSecretName(deployment, webhookCertDir)).To(Equal("toto-webhook-tls"))
		Expect(MountedSecretName(deployment, webhookCertDir+"/")).To(Equal("toto-webhook-tls"))
	})

	It("is empty without a deployment, a directory, a mount there, or a Secret behind it", func() {
		deployment := operatorDeploymentWithMount("toto-webhook-tls")
		Expect(MountedSecretName(nil, webhookCertDir)).To(BeEmpty())
		Expect(MountedSecretName(deployment, "")).To(BeEmpty())
		Expect(MountedSecretName(deployment, "/run/secrets/elsewhere")).To(BeEmpty())
		Expect(MountedSecretName(deployment, "/controller")).To(BeEmpty())
	})

	It("falls back to the configured name", func() {
		pki := pkiEnvironmentTemplate
		pki.CertDir = webhookCertDir
		Expect(pki.webhookSecretName(nil)).To(Equal(pki.SecretName))
		Expect(pki.webhookSecretName(operatorDeploymentWithMount(""))).To(Equal(pki.SecretName))
		Expect(pki.webhookSecretName(operatorDeploymentWithMount("toto-webhook-tls"))).
			To(Equal("toto-webhook-tls"))
	})
})

var _ = Describe("Webhook certificate DNS names", func() {
	It("are the configured names, the historical one included, and nothing else", func() {
		pki := pkiEnvironmentTemplate
		pki.ServiceName = "cnpg-toto-webhook-service"
		pki.AlternativeServiceNames = []string{"cnpg-webhook-service", "", "cnpg-toto-webhook-service"}

		Expect(pki.webhookAltDNSNames()).To(Equal([]string{
			"cnpg-toto-webhook-service." + operatorNamespaceName + ".svc",
			"cnpg-webhook-service." + operatorNamespaceName + ".svc",
		}))
	})
})

var _ = Describe("Webhook certificate from the operator deployment", func() {
	It("uses the mounted Secret, the configured names, and the deployment as owner", func(ctx SpecContext) {
		pki := pkiEnvironmentTemplate
		pki.CertDir = webhookCertDir
		pki.OperatorDeploymentName = "cnpg-toto"
		pki.ServiceName = "cnpg-toto-webhook-service"
		pki.AlternativeServiceNames = []string{"cnpg-webhook-service"}
		pki.ManageWebhookConfigurations = false

		kubeClient := generateFakeClient()
		Expect(kubeClient.Create(ctx, operatorDeploymentWithMount("toto-webhook-tls"))).To(Succeed())
		// Another Service exposing the webhook is not in the certificate: its
		// name is given with WEBHOOK_SERVICE_NAME
		Expect(kubeClient.Create(ctx, &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "cnpg-toto-other", Namespace: operatorNamespaceName},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app.kubernetes.io/name": "cnpg-toto"},
				Ports:    []corev1.ServicePort{{Port: 443, TargetPort: intstr.FromInt32(9443)}},
			},
		})).To(Succeed())

		ca, err := pki.ensureRootCACertificate(ctx, kubeClient)
		Expect(err).ToNot(HaveOccurred())
		Expect(ca.OwnerReferences).To(HaveLen(1))
		Expect(ca.OwnerReferences[0].Name).To(Equal("cnpg-toto"))

		secret, err := pki.ensureCertificate(ctx, kubeClient, ca)
		Expect(err).ToNot(HaveOccurred())
		Expect(secret.Name).To(Equal("toto-webhook-tls"))
		Expect(secret.OwnerReferences).To(HaveLen(1))
		Expect(secret.OwnerReferences[0].Name).To(Equal("cnpg-toto"))
		Expect(kubeClient.Get(ctx, client.ObjectKeyFromObject(secret), &corev1.Secret{})).To(Succeed())
		pair, err := ParseServerSecret(secret)
		Expect(err).ToNot(HaveOccurred())
		cert, err := pair.ParseCertificate()
		Expect(err).ToNot(HaveOccurred())
		Expect(cert.DNSNames).To(ConsistOf(
			"cnpg-toto-webhook-service."+operatorNamespaceName+".svc",
			"cnpg-webhook-service."+operatorNamespaceName+".svc",
		))

		By("keeping the certificate when nothing changed")
		again, err := pki.ensureCertificate(ctx, kubeClient, ca)
		Expect(err).ToNot(HaveOccurred())
		Expect(again.Data).To(Equal(secret.Data))

		By("renewing the certificate when the configured names change")
		pki.ServiceName = "cnpg-toto-other"
		renewed, err := pki.ensureCertificate(ctx, kubeClient, ca)
		Expect(err).ToNot(HaveOccurred())
		pair, err = ParseServerSecret(renewed)
		Expect(err).ToNot(HaveOccurred())
		cert, err = pair.ParseCertificate()
		Expect(err).ToNot(HaveOccurred())
		Expect(cert.DNSNames).To(ContainElement("cnpg-toto-other." + operatorNamespaceName + ".svc"))
	})

	It("renews an existing Secret even when the deployment cannot be read, "+
		"and refuses to create one without owner", func(ctx SpecContext) {
		pki := pkiEnvironmentTemplate
		pki.OperatorDeploymentName = "cnpg-toto"
		pki.ManageWebhookConfigurations = false
		kubeClient := generateFakeClient()

		ca, err := pki.ensureRootCACertificate(ctx, kubeClient)
		Expect(err).To(MatchError(ContainSubstring("OPERATOR_DEPLOYMENT_NAME")))
		Expect(ca).To(BeNil())

		Expect(kubeClient.Create(ctx, operatorDeploymentWithMount(""))).To(Succeed())
		ca, err = pki.ensureRootCACertificate(ctx, kubeClient)
		Expect(err).ToNot(HaveOccurred())
		secret, err := pki.ensureCertificate(ctx, kubeClient, ca)
		Expect(err).ToNot(HaveOccurred())
		Expect(secret.Name).To(Equal(pki.SecretName))

		Expect(kubeClient.Delete(ctx, operatorDeploymentWithMount(""))).To(Succeed())
		again, err := pki.ensureCertificate(ctx, kubeClient, ca)
		Expect(err).ToNot(HaveOccurred())
		Expect(again.Name).To(Equal(pki.SecretName))

		Expect(kubeClient.Delete(ctx, again)).To(Succeed())
		_, err = pki.ensureCertificate(ctx, kubeClient, ca)
		Expect(err).To(MatchError(ContainSubstring("OPERATOR_DEPLOYMENT_NAME")))
	})

	// Without the deployment the mounted Secret is not known: the Secret of
	// the configured name must not be renewed in its place
	It("keeps the certificate when the deployment cannot be read for a while", func(ctx SpecContext) {
		pki := pkiEnvironmentTemplate
		pki.CertDir = webhookCertDir
		pki.OperatorDeploymentName = "cnpg-toto"
		pki.ServiceName = "cnpg-toto-webhook-service"
		pki.ManageWebhookConfigurations = false

		failing := false
		kubeClient := fakeClientBuilder(controllerScheme.BuildWithAllKnownScheme()).
			WithInterceptorFuncs(interceptor.Funcs{
				Get: func(
					ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object,
					opts ...client.GetOption,
				) error {
					if _, ok := obj.(*appsv1.Deployment); ok && failing {
						return apierrors.NewTimeoutError("get deployment", 1)
					}
					return c.Get(ctx, key, obj, opts...)
				},
			}).Build()
		// The certificate is in the Secret of the configured name: the one
		// looked up when the deployment, and its mount, cannot be read
		Expect(kubeClient.Create(ctx, operatorDeploymentWithMount(pki.SecretName))).To(Succeed())
		ca, err := pki.ensureRootCACertificate(ctx, kubeClient)
		Expect(err).ToNot(HaveOccurred())

		secret, err := pki.ensureCertificate(ctx, kubeClient, ca)
		Expect(err).ToNot(HaveOccurred())

		// The names changed: the certificate would be reissued, were the
		// deployment readable
		failing = true
		pki.ServiceName = "cnpg-toto-renamed"
		kept, err := pki.ensureCertificate(ctx, kubeClient, ca)
		Expect(err).ToNot(HaveOccurred())
		Expect(kept.Data).To(Equal(secret.Data))

		failing = false
		renewed, err := pki.ensureCertificate(ctx, kubeClient, ca)
		Expect(err).ToNot(HaveOccurred())
		Expect(renewed.Data).ToNot(Equal(secret.Data))
	})
})
