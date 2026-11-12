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
	"errors"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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

func serviceSelecting(name string, selector map[string]string, targetPort intstr.IntOrString) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: operatorNamespaceName},
		Spec: corev1.ServiceSpec{
			Selector: selector,
			Ports:    []corev1.ServicePort{{Port: 443, TargetPort: targetPort}},
		},
	}
}

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
	var pki PublicKeyInfrastructure
	var deployment *appsv1.Deployment
	podSelector := map[string]string{"app.kubernetes.io/name": "cnpg-toto"}

	BeforeEach(func() {
		pki = pkiEnvironmentTemplate
		pki.ServiceName = "cnpg-toto-webhook-service"
		pki.AlternativeServiceNames = []string{"cnpg-webhook-service"}
		pki.WebhookPort = 9443
		deployment = operatorDeploymentWithMount("cnpg-toto-webhook-cert")
	})

	It("always carry the configured names, the historical one included", func(ctx SpecContext) {
		kubeClient := generateFakeClient()
		Expect(pki.webhookAltDNSNames(ctx, kubeClient, nil)).To(Equal([]string{
			"cnpg-toto-webhook-service." + operatorNamespaceName + ".svc",
			"cnpg-webhook-service." + operatorNamespaceName + ".svc",
		}))
	})

	It("add the Services selecting the pods of the deployment on the webhook port", func(ctx SpecContext) {
		kubeClient := generateFakeClient()
		Expect(kubeClient.Create(ctx, serviceSelecting("by-number", podSelector, intstr.FromInt32(9443)))).
			To(Succeed())
		Expect(kubeClient.Create(ctx, serviceSelecting("by-name", podSelector, intstr.FromString("webhook-server")))).
			To(Succeed())
		Expect(kubeClient.Create(ctx, serviceSelecting("metrics", podSelector, intstr.FromInt32(8080)))).
			To(Succeed())
		Expect(kubeClient.Create(ctx, serviceSelecting("other-pods",
			map[string]string{"app.kubernetes.io/name": "cnpg-tata"}, intstr.FromInt32(9443)))).To(Succeed())
		Expect(kubeClient.Create(ctx, serviceSelecting("no-selector", nil, intstr.FromInt32(9443)))).To(Succeed())
		Expect(kubeClient.Create(ctx, serviceSelecting("unknown-port-name", podSelector,
			intstr.FromString("https")))).To(Succeed())
		elsewhere := serviceSelecting("elsewhere", podSelector, intstr.FromInt32(9443))
		elsewhere.Namespace = "elsewhere"
		Expect(kubeClient.Create(ctx, elsewhere)).To(Succeed())

		Expect(pki.servicesSelectingDeployment(ctx, kubeClient, deployment)).
			To(ConsistOf("by-number", "by-name"))
	})

	It("skip the Services without a deployment or a webhook port", func(ctx SpecContext) {
		kubeClient := generateFakeClient()
		Expect(kubeClient.Create(ctx, serviceSelecting("by-number", podSelector, intstr.FromInt32(9443)))).
			To(Succeed())
		Expect(pki.servicesSelectingDeployment(ctx, kubeClient, nil)).To(BeEmpty())
		pki.WebhookPort = 0
		Expect(pki.servicesSelectingDeployment(ctx, kubeClient, deployment)).To(BeEmpty())
	})

	It("skip the Services when they cannot be listed", func(ctx SpecContext) {
		kubeClient := generateFakeClientWithListError(errors.New("services is forbidden"))
		Expect(pki.servicesSelectingDeployment(ctx, kubeClient, deployment)).To(BeEmpty())
		Expect(pki.webhookAltDNSNames(ctx, kubeClient, deployment)).To(HaveLen(2))
	})

	It("add the Services the managed webhook configurations point at, in the operator namespace",
		func(ctx SpecContext) {
			kubeClient := generateFakeClient()
			mutating := mutatingWebhookTemplate
			mutating.Webhooks[0].ClientConfig.Service = &admissionregistrationv1.ServiceReference{
				Namespace: operatorNamespaceName, Name: "platform-webhook",
			}
			validating := validatingWebhookTemplate
			validating.Webhooks = []admissionregistrationv1.ValidatingWebhook{
				{ClientConfig: admissionregistrationv1.WebhookClientConfig{
					Service: &admissionregistrationv1.ServiceReference{Namespace: "elsewhere", Name: "foreign"},
				}},
				{ClientConfig: admissionregistrationv1.WebhookClientConfig{URL: ptrString("https://example")}},
				{ClientConfig: admissionregistrationv1.WebhookClientConfig{
					Service: &admissionregistrationv1.ServiceReference{Namespace: operatorNamespaceName, Name: "platform-webhook"},
				}},
			}
			Expect(kubeClient.Create(ctx, &mutating)).To(Succeed())
			Expect(kubeClient.Create(ctx, &validating)).To(Succeed())

			Expect(pki.servicesOfWebhookConfigurations(ctx, kubeClient)).
				To(Equal([]string{"platform-webhook", "platform-webhook"}))
			Expect(pki.webhookAltDNSNames(ctx, kubeClient, deployment)).To(Equal([]string{
				"cnpg-toto-webhook-service." + operatorNamespaceName + ".svc",
				"cnpg-webhook-service." + operatorNamespaceName + ".svc",
				"platform-webhook." + operatorNamespaceName + ".svc",
			}))
		})

	It("skip the webhook configurations when they are not managed or not found", func(ctx SpecContext) {
		kubeClient := generateFakeClient()
		Expect(pki.servicesOfWebhookConfigurations(ctx, kubeClient)).To(BeEmpty())
		mutating := mutatingWebhookTemplate
		mutating.Webhooks[0].ClientConfig.Service = &admissionregistrationv1.ServiceReference{
			Namespace: operatorNamespaceName, Name: "platform-webhook",
		}
		Expect(kubeClient.Create(ctx, &mutating)).To(Succeed())
		pki.ManageWebhookConfigurations = false
		Expect(pki.servicesOfWebhookConfigurations(ctx, kubeClient)).To(BeEmpty())
	})
})

// Goal: the serving certificate is stored in the Secret the operator Pod
// mounts and covers every name the webhook server is reachable through, and
// is renewed when a new name appears
var _ = Describe("Webhook certificate from the operator deployment", func() {
	It("uses the mounted Secret, the discovered names, and the deployment as owner", func(ctx SpecContext) {
		pki := pkiEnvironmentTemplate
		pki.CertDir = webhookCertDir
		pki.OperatorDeploymentName = "cnpg-toto"
		pki.ServiceName = "cnpg-toto-webhook-service"
		pki.AlternativeServiceNames = []string{"cnpg-webhook-service"}
		pki.WebhookPort = 9443
		pki.ManageWebhookConfigurations = false
		podSelector := map[string]string{"app.kubernetes.io/name": "cnpg-toto"}

		kubeClient := generateFakeClient()
		Expect(kubeClient.Create(ctx, operatorDeploymentWithMount("toto-webhook-tls"))).To(Succeed())
		Expect(kubeClient.Create(ctx, serviceSelecting("cnpg-toto-service", podSelector, intstr.FromInt32(9443)))).
			To(Succeed())

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
			"cnpg-toto-service."+operatorNamespaceName+".svc",
		))

		By("renewing the certificate when another Service exposes the webhook")
		Expect(kubeClient.Create(ctx, serviceSelecting("cnpg-toto-other", podSelector, intstr.FromInt32(9443)))).
			To(Succeed())
		renewed, err := pki.ensureCertificate(ctx, kubeClient, ca)
		Expect(err).ToNot(HaveOccurred())
		Expect(renewed.Name).To(Equal("toto-webhook-tls"))
		pair, err = ParseServerSecret(renewed)
		Expect(err).ToNot(HaveOccurred())
		cert, err = pair.ParseCertificate()
		Expect(err).ToNot(HaveOccurred())
		Expect(cert.DNSNames).To(ContainElement("cnpg-toto-other." + operatorNamespaceName + ".svc"))

		By("keeping the certificate when nothing changed")
		again, err := pki.ensureCertificate(ctx, kubeClient, ca)
		Expect(err).ToNot(HaveOccurred())
		Expect(again.Data).To(Equal(renewed.Data))
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
})

func ptrString(s string) *string { return &s }

// generateFakeClientWithListError returns a client whose List calls fail
func generateFakeClientWithListError(listErr error) client.Client {
	scheme := controllerScheme.BuildWithAllKnownScheme()
	return fakeClientBuilder(scheme).WithInterceptorFuncs(interceptor.Funcs{
		List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error {
			return listErr
		},
	}).Build()
}
