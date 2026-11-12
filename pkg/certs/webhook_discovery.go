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
	"fmt"
	"path/filepath"
	"slices"

	"github.com/cloudnative-pg/machinery/pkg/log"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// operatorDeployment fetches the operator deployment, by name when known, by
// label otherwise. The error is returned, not logged: the callers decide
// whether the deployment is required.
func (pki PublicKeyInfrastructure) operatorDeployment(
	ctx context.Context,
	kubeClient client.Client,
) (*appsv1.Deployment, error) {
	return GetOperatorDeployment(ctx, kubeClient, pki.OperatorNamespace,
		pki.OperatorDeploymentName, pki.OperatorDeploymentLabelSelector)
}

// webhookSecretName returns the name of the Secret holding the serving
// certificate: the Secret mounted at the certificate directory of the
// deployment when one is, the configured name otherwise
func (pki PublicKeyInfrastructure) webhookSecretName(deployment *appsv1.Deployment) string {
	if name := mountedSecretName(deployment, pki.CertDir); name != "" {
		return name
	}
	return pki.SecretName
}

// mountedSecretName returns the name of the Secret a container of the
// deployment mounts at the given directory, or an empty string when none does
func mountedSecretName(deployment *appsv1.Deployment, mountPath string) string {
	if deployment == nil || mountPath == "" {
		return ""
	}
	mountPath = filepath.Clean(mountPath)

	secretVolumes := make(map[string]string)
	for _, volume := range deployment.Spec.Template.Spec.Volumes {
		if volume.Secret != nil {
			secretVolumes[volume.Name] = volume.Secret.SecretName
		}
	}

	for _, container := range deployment.Spec.Template.Spec.Containers {
		for _, mount := range container.VolumeMounts {
			if filepath.Clean(mount.MountPath) != mountPath {
				continue
			}
			if name := secretVolumes[mount.Name]; name != "" {
				return name
			}
		}
	}
	return ""
}

// webhookAltDNSNames returns the DNS names the serving certificate must carry,
// sorted and without duplicates: the configured Service name and the
// alternative ones, the Services of the operator namespace selecting the pods
// of the deployment on the webhook port, and the Services the managed webhook
// configurations point at. The discovered names are best effort: what cannot
// be read is logged and skipped, the configured names are always present.
func (pki PublicKeyInfrastructure) webhookAltDNSNames(
	ctx context.Context,
	kubeClient client.Client,
	deployment *appsv1.Deployment,
) []string {
	serviceNames := append([]string{pki.ServiceName}, pki.AlternativeServiceNames...)
	serviceNames = append(serviceNames, pki.servicesSelectingDeployment(ctx, kubeClient, deployment)...)
	serviceNames = append(serviceNames, pki.servicesOfWebhookConfigurations(ctx, kubeClient)...)

	dnsNames := make([]string, 0, len(serviceNames))
	for _, name := range serviceNames {
		if name == "" {
			continue
		}
		dnsNames = append(dnsNames, fmt.Sprintf("%v.%v.svc", name, pki.OperatorNamespace))
	}
	slices.Sort(dnsNames)
	return slices.Compact(dnsNames)
}

// servicesSelectingDeployment returns the names of the Services of the
// operator namespace whose selector matches the pod template of the
// deployment and which expose the webhook port
func (pki PublicKeyInfrastructure) servicesSelectingDeployment(
	ctx context.Context,
	kubeClient client.Client,
	deployment *appsv1.Deployment,
) []string {
	if deployment == nil || pki.WebhookPort == 0 {
		return nil
	}

	services := &corev1.ServiceList{}
	if err := kubeClient.List(ctx, services, client.InNamespace(pki.OperatorNamespace)); err != nil {
		log.FromContext(ctx).Info("Cannot list the Services of the operator namespace, "+
			"the webhook certificate carries the configured names only",
			"namespace", pki.OperatorNamespace, "error", err.Error())
		return nil
	}

	podTemplate := deployment.Spec.Template.Spec
	podLabels := deployment.Spec.Template.Labels
	var names []string
	for _, service := range services.Items {
		if !selectorMatches(service.Spec.Selector, podLabels) {
			continue
		}
		for _, port := range service.Spec.Ports {
			if targetsPort(port.TargetPort, pki.WebhookPort, podTemplate.Containers) {
				names = append(names, service.Name)
				break
			}
		}
	}
	return names
}

// selectorMatches is true when the selector is not empty and every one of its
// labels is carried by the pod labels
func selectorMatches(selector, podLabels map[string]string) bool {
	if len(selector) == 0 {
		return false
	}
	for key, value := range selector {
		if podLabels[key] != value {
			return false
		}
	}
	return true
}

// targetsPort is true when the target port of a Service port is the given
// port number, directly or through the name of a container port
func targetsPort(target intstr.IntOrString, port int, containers []corev1.Container) bool {
	switch target.Type {
	case intstr.Int:
		return int(target.IntVal) == port
	case intstr.String:
		for _, container := range containers {
			for _, containerPort := range container.Ports {
				if containerPort.Name == target.StrVal && int(containerPort.ContainerPort) == port {
					return true
				}
			}
		}
	}
	return false
}

// servicesOfWebhookConfigurations returns the names of the Services of the
// operator namespace the managed webhook configurations point at
func (pki PublicKeyInfrastructure) servicesOfWebhookConfigurations(
	ctx context.Context,
	kubeClient client.Client,
) []string {
	if !pki.ManageWebhookConfigurations {
		return nil
	}

	var clientConfigs []admissionregistrationv1.WebhookClientConfig

	mutating := &admissionregistrationv1.MutatingWebhookConfiguration{}
	if err := kubeClient.Get(ctx, types.NamespacedName{Name: pki.MutatingWebhookConfigurationName},
		mutating); err == nil {
		for _, webhook := range mutating.Webhooks {
			clientConfigs = append(clientConfigs, webhook.ClientConfig)
		}
	}

	validating := &admissionregistrationv1.ValidatingWebhookConfiguration{}
	if err := kubeClient.Get(ctx, types.NamespacedName{Name: pki.ValidatingWebhookConfigurationName},
		validating); err == nil {
		for _, webhook := range validating.Webhooks {
			clientConfigs = append(clientConfigs, webhook.ClientConfig)
		}
	}

	var names []string
	for _, clientConfig := range clientConfigs {
		if clientConfig.Service != nil && clientConfig.Service.Namespace == pki.OperatorNamespace {
			names = append(names, clientConfig.Service.Name)
		}
	}
	return names
}
