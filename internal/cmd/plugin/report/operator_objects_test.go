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

package report

import (
	"os"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/internal/cmd/plugin"
	controllerScheme "github.com/cloudnative-pg/cloudnative-pg/internal/scheme"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const operatorsNamespace = "awesome-operators"

// operatorDeployment is a Deployment shaped like the operator's: the manager
// container running the controller, the given labels on the pods, the given
// environment, and the webhook Secret mounted at the certificate directory
func operatorDeployment(name string, labels map[string]string, env map[string]string) *appsv1.Deployment {
	envVars := make([]corev1.EnvVar, 0, len(env)+1)
	for key, value := range env {
		envVars = append(envVars, corev1.EnvVar{Name: key, Value: value})
	}
	envVars = append(envVars, corev1.EnvVar{
		Name:      "OPERATOR_NAMESPACE",
		ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.namespace"}},
	})
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: operatorsNamespace, Labels: labels, UID: types.UID("uid-" + name),
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:    "manager",
						Command: []string{"/manager"},
						Args:    []string{"controller", "--leader-elect", "--config-map-name=" + name + "-config"},
						Env:     envVars,
						VolumeMounts: []corev1.VolumeMount{
							{Name: "webhook-certificates", MountPath: "/run/secrets/cnpg.io/webhook"},
						},
					}},
					Volumes: []corev1.Volume{{
						Name:         "webhook-certificates",
						VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: name + "-webhook-cert"}},
					}},
				},
			},
		},
	}
}

func webhookConfigurations(name, serviceName string) (
	*admissionregistrationv1.MutatingWebhookConfiguration,
	*admissionregistrationv1.ValidatingWebhookConfiguration,
) {
	clientConfig := admissionregistrationv1.WebhookClientConfig{
		Service: &admissionregistrationv1.ServiceReference{Name: serviceName, Namespace: operatorsNamespace},
	}
	rules := []admissionregistrationv1.RuleWithOperations{{
		Rule: admissionregistrationv1.Rule{APIGroups: []string{apiv1.SchemeGroupVersion.Group}},
	}}
	return &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: name + "-mutating-webhook-configuration"},
		Webhooks: []admissionregistrationv1.MutatingWebhook{
			{Name: "mcluster.cnpg.io", ClientConfig: clientConfig, Rules: rules},
		},
	}, &admissionregistrationv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: name + "-validating-webhook-configuration"},
		Webhooks: []admissionregistrationv1.ValidatingWebhook{
			{Name: "vcluster.cnpg.io", ClientConfig: clientConfig, Rules: rules},
		},
	}
}

func webhookService(name string, selector map[string]string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: operatorsNamespace},
		Spec:       corev1.ServiceSpec{Selector: selector, Ports: []corev1.ServicePort{{Port: 443}}},
	}
}

func ownedSecret(name string, owner *appsv1.Deployment) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: operatorsNamespace,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "apps/v1", Kind: "Deployment", Name: owner.Name, UID: owner.UID,
			}},
		},
	}
}

func names[T any](items []T, name func(T) string) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, name(item))
	}
	return result
}

// Goal: the report starts from the operator Deployment, by name or by discovery,
// and resolves every other object the way the operator does
var _ = Describe("Operator objects from the Deployment", func() {
	var (
		savedClient    ctrlclient.Client
		savedNamespace string
		toto, tata     *appsv1.Deployment
		totoLabels     = map[string]string{"app.kubernetes.io/name": "cnpg-toto"}
		tataLabels     = map[string]string{"app.kubernetes.io/name": "cnpg-tata"}
	)

	BeforeEach(func() {
		savedClient, savedNamespace = plugin.Client, plugin.Namespace
		plugin.Namespace = operatorsNamespace
		toto = operatorDeployment("cnpg-toto", totoLabels, map[string]string{
			"OPERATOR_DEPLOYMENT_NAME": "cnpg-toto", "WATCH_NAMESPACE": "toto",
		})
		tata = operatorDeployment("cnpg-tata", tataLabels, map[string]string{
			"OPERATOR_DEPLOYMENT_NAME": "cnpg-tata", "WATCH_NAMESPACE": "tata",
		})
	})

	AfterEach(func() {
		plugin.Client, plugin.Namespace = savedClient, savedNamespace
	})

	withObjects := func(objects ...ctrlclient.Object) {
		plugin.Client = fake.NewClientBuilder().
			WithScheme(controllerScheme.BuildWithAllKnownScheme()).
			WithObjects(objects...).
			Build()
	}

	Context("the Deployment", func() {
		It("is fetched by name, whatever its labels, and a missing name is a not-found", func(ctx SpecContext) {
			withObjects(toto, tata)
			deployment, err := getOperatorDeployment(ctx, "cnpg-tata")
			Expect(err).ToNot(HaveOccurred())
			Expect(deployment.Name).To(Equal("cnpg-tata"))

			_, err = getOperatorDeployment(ctx, "cnpg-nope")
			Expect(err).To(MatchError(errNoOperatorDeployment))
			Expect(err.Error()).To(ContainSubstring("'cnpg-nope'"))
		})

		It("is the only one running the operator, by label or by shape, when no name is given",
			func(ctx SpecContext) {
				stock := operatorDeployment("cnpg-controller-manager",
					map[string]string{"app.kubernetes.io/name": "cloudnative-pg"}, nil)
				other := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: "something-else", Namespace: operatorsNamespace},
					Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "app", Command: []string{"/app"}}},
					}}},
				}
				withObjects(stock, other)
				deployment, err := getOperatorDeployment(ctx, "")
				Expect(err).ToNot(HaveOccurred())
				Expect(deployment.Name).To(Equal("cnpg-controller-manager"))

				withObjects(toto, other)
				deployment, err = getOperatorDeployment(ctx, "")
				Expect(err).ToNot(HaveOccurred())
				Expect(deployment.Name).To(Equal("cnpg-toto"))

				withObjects(other)
				_, err = getOperatorDeployment(ctx, "")
				Expect(err).To(MatchError(errNoOperatorDeployment))
			})

		It("lists the candidates when several run the operator", func(ctx SpecContext) {
			withObjects(toto, tata)
			_, err := getOperatorDeployment(ctx, "")
			Expect(err).To(MatchError(ContainSubstring("several operator deployments in namespace 'awesome-operators': " +
				"cnpg-tata, cnpg-toto")))
			Expect(err.Error()).To(ContainSubstring("--deployment-name"))
		})

		DescribeTable("is recognised",
			func(deployment *appsv1.Deployment, expected bool) {
				Expect(isOperatorDeployment(*deployment)).To(Equal(expected))
			},
			Entry("by the operator label", operatorDeployment("x",
				map[string]string{"app.kubernetes.io/name": "cloudnative-pg"}, nil), true),
			Entry("by the manager container running the controller", operatorDeployment("x", nil, nil), true),
			Entry("not by another label alone", &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app.kubernetes.io/name": "cnpg-toto"}},
			}, false),
			Entry("not by a manager container running something else", &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "manager", Command: []string{"/manager"}, Args: []string{"instance"}}},
				}}},
			}, false),
		)
	})

	Context("the Pods", func() {
		It("are the ones the Deployment selects", func(ctx SpecContext) {
			totoPod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
				Name: "cnpg-toto-1", Namespace: operatorsNamespace, Labels: totoLabels,
			}}
			tataPod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
				Name: "cnpg-tata-1", Namespace: operatorsNamespace, Labels: tataLabels,
			}}
			withObjects(toto, tata, totoPod, tataPod)
			pods, err := getOperatorPods(ctx, *toto)
			Expect(err).ToNot(HaveOccurred())
			Expect(names(pods, func(p corev1.Pod) string { return p.Name })).To(Equal([]string{"cnpg-toto-1"}))
		})

		It("need a selector", func(ctx SpecContext) {
			withObjects()
			_, err := getOperatorPods(ctx, appsv1.Deployment{})
			Expect(err).To(MatchError(ContainSubstring("selects no pod")))
		})
	})

	Context("the configuration of the operator", func() {
		It("is read from the manager container, not from this process", func() {
			Expect(os.Setenv("CA_SECRET_NAME", "from-the-plugin-process")).To(Succeed())
			DeferCleanup(os.Unsetenv, "CA_SECRET_NAME")

			conf := operatorConfiguration(*toto)
			Expect(conf.OperatorDeploymentName).To(Equal("cnpg-toto"))
			Expect(conf.OperatorNamespace).To(Equal(operatorsNamespace))
			Expect(conf.GetCaSecretName()).To(Equal("cnpg-toto-ca-secret"))
			Expect(conf.GetMutatingWebhookConfigurationName()).To(Equal("cnpg-toto-mutating-webhook-configuration"))
			Expect(conf.GetLeaderElectionID()).To(Equal("cnpg-toto.cnpg.io"))
		})

		It("resolves the historical names for a stock Deployment", func() {
			stock := operatorDeployment("cnpg-controller-manager",
				map[string]string{"app.kubernetes.io/name": "cloudnative-pg"}, nil)
			conf := operatorConfiguration(*stock)
			Expect(conf.GetCaSecretName()).To(Equal("cnpg-ca-secret"))
			Expect(conf.GetWebhookSecretName()).To(Equal("cnpg-webhook-cert"))
			Expect(conf.GetMutatingWebhookConfigurationName()).To(Equal("cnpg-mutating-webhook-configuration"))
		})

		It("takes the webhook Secret from the mount, then from the names", func() {
			conf := operatorConfiguration(*toto)
			Expect(webhookSecretName(*toto, conf)).To(Equal("cnpg-toto-webhook-cert"))

			renamed := toto.DeepCopy()
			renamed.Spec.Template.Spec.Volumes[0].Secret.SecretName = "toto-tls"
			Expect(webhookSecretName(*renamed, conf)).To(Equal("toto-tls"))

			external := toto.DeepCopy()
			external.Spec.Template.Spec.Containers[0].Env = append(external.Spec.Template.Spec.Containers[0].Env,
				corev1.EnvVar{Name: "WEBHOOK_CERT_DIR", Value: "/run/secrets/webhook"})
			external.Spec.Template.Spec.Containers[0].VolumeMounts[0].MountPath = "/run/secrets/webhook"
			external.Spec.Template.Spec.Volumes[0].Secret.SecretName = "cert-manager-tls"
			Expect(webhookSecretName(*external, operatorConfiguration(*external))).To(Equal("cert-manager-tls"))

			unmounted := toto.DeepCopy()
			unmounted.Spec.Template.Spec.Volumes = nil
			Expect(webhookSecretName(*unmounted, conf)).To(Equal("cnpg-toto-webhook-cert"))
		})
	})

	Context("the Secrets", func() {
		It("are the ones named by the configuration and the ones owned by the Deployment", func(ctx SpecContext) {
			withObjects(toto, tata,
				ownedSecret("cnpg-toto-ca-secret", toto),
				ownedSecret("cnpg-toto-webhook-cert", toto),
				ownedSecret("cnpg-toto-old-cert", toto),
				ownedSecret("cnpg-tata-ca-secret", tata),
				&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "cnpg-toto-config", Namespace: operatorsNamespace}},
			)
			toto.Spec.Template.Spec.Containers[0].Args = append(toto.Spec.Template.Spec.Containers[0].Args,
				"--secret-name=cnpg-toto-config")
			secrets, err := getOperatorSecrets(ctx, *toto)
			Expect(err).ToNot(HaveOccurred())
			Expect(names(secrets, func(s corev1.Secret) string { return s.Name })).To(ConsistOf(
				"cnpg-toto-webhook-cert", "cnpg-toto-ca-secret", "cnpg-toto-config", "cnpg-toto-old-cert"))
		})
	})

	Context("the webhook configurations and Services", func() {
		It("are the ones of this operator, not of the other operator of the namespace", func(ctx SpecContext) {
			totoMutating, totoValidating := webhookConfigurations("cnpg-toto", "cnpg-toto-webhook-service")
			tataMutating, tataValidating := webhookConfigurations("cnpg-tata", "cnpg-tata-webhook-service")
			platformMutating, platformValidating := webhookConfigurations("platform", "cnpg-toto-extra")
			elsewhereMutating, _ := webhookConfigurations("elsewhere", "cnpg-webhook-service")
			elsewhereMutating.Webhooks[0].ClientConfig.Service.Namespace = "elsewhere"
			withObjects(toto, tata,
				totoMutating, totoValidating, tataMutating, tataValidating,
				platformMutating, platformValidating, elsewhereMutating,
				webhookService("cnpg-toto-webhook-service", totoLabels),
				webhookService("cnpg-toto-extra", totoLabels),
				webhookService("cnpg-tata-webhook-service", tataLabels),
			)

			mutating, validating, err := getWebhooks(ctx, *toto, true)
			Expect(err).ToNot(HaveOccurred())
			Expect(names(mutating.Items, func(m admissionregistrationv1.MutatingWebhookConfiguration) string {
				return m.Name
			})).To(ConsistOf("cnpg-toto-mutating-webhook-configuration", "platform-mutating-webhook-configuration"))
			Expect(names(validating.Items, func(v admissionregistrationv1.ValidatingWebhookConfiguration) string {
				return v.Name
			})).To(ConsistOf("cnpg-toto-validating-webhook-configuration", "platform-validating-webhook-configuration"))

			services, err := getWebhookServices(ctx, mutating, validating)
			Expect(err).ToNot(HaveOccurred())
			Expect(names(services, func(s corev1.Service) string { return s.Name })).
				To(ConsistOf("cnpg-toto-webhook-service", "cnpg-toto-extra"))
		})

		It("are redacted unless asked otherwise", func(ctx SpecContext) {
			totoMutating, totoValidating := webhookConfigurations("cnpg-toto", "cnpg-toto-webhook-service")
			totoMutating.Webhooks[0].ClientConfig.CABundle = []byte("ca")
			withObjects(toto, totoMutating, totoValidating, webhookService("cnpg-toto-webhook-service", totoLabels))

			mutating, _, err := getWebhooks(ctx, *toto, false)
			Expect(err).ToNot(HaveOccurred())
			Expect(mutating.Items[0].Webhooks[0].ClientConfig.CABundle).ToNot(Equal([]byte("ca")))

			mutating, _, err = getWebhooks(ctx, *toto, true)
			Expect(err).ToNot(HaveOccurred())
			Expect(mutating.Items[0].Webhooks[0].ClientConfig.CABundle).To(Equal([]byte("ca")))
		})

		It("are an error when none is found", func(ctx SpecContext) {
			withObjects(toto)
			_, _, err := getWebhooks(ctx, *toto, true)
			Expect(err).To(MatchError(ContainSubstring("can't find the webhooks")))
		})
	})
})
