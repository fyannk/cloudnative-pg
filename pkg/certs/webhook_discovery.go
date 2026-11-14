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

	appsv1 "k8s.io/api/apps/v1"
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

// webhookAltDNSNames returns the DNS names the serving certificate carries,
// sorted and without duplicates: the configured Service name and the
// alternative ones. A webhook Service with another name is declared with
// WEBHOOK_SERVICE_NAME
func (pki PublicKeyInfrastructure) webhookAltDNSNames() []string {
	serviceNames := append([]string{pki.ServiceName}, pki.AlternativeServiceNames...)

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
