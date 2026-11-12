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
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/cloudnative-pg/machinery/pkg/log"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cloudnative-pg/cloudnative-pg/internal/cmd/plugin"
	"github.com/cloudnative-pg/cloudnative-pg/internal/configuration"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/certs"
)

const (
	defaultConfigSecretName = "cnpg-controller-manager-config" // #nosec
	defaultConfigmapName    = "cnpg-controller-manager-config"
	labelOperatorNameKey    = "app.kubernetes.io/name"
	labelOperatorName       = "cloudnative-pg"
	labelOperatorKeyPrefix  = "operators.coreos.com/cloudnative-pg."
	managerContainerName    = "manager"
	managerCommand          = "/manager"
	managerControllerArg    = "controller"
)

var errNoOperatorDeployment = fmt.Errorf("no deployment found")

func getLabelOperatorsNamespace() string {
	return labelOperatorKeyPrefix + plugin.Namespace
}

// getOperatorDeployment returns the operator Deployment: the one named when a
// name is given, fetched with get; otherwise the only Deployment of the
// namespace running the operator, found by its label or by the shape of its
// manager container. None is errNoOperatorDeployment, several is an error
// listing them.
func getOperatorDeployment(ctx context.Context, deploymentName string) (appsv1.Deployment, error) {
	if deploymentName != "" {
		var deployment appsv1.Deployment
		err := plugin.Client.Get(ctx,
			types.NamespacedName{Namespace: plugin.Namespace, Name: deploymentName}, &deployment)
		if apierrors.IsNotFound(err) {
			return appsv1.Deployment{}, fmt.Errorf("%w: deployment '%s' in namespace '%s'",
				errNoOperatorDeployment, deploymentName, plugin.Namespace)
		}
		if err != nil {
			return appsv1.Deployment{},
				fmt.Errorf("could not get deployment '%s' in namespace '%s': %w",
					deploymentName, plugin.Namespace, err)
		}
		return deployment, nil
	}

	deploymentList := &appsv1.DeploymentList{}
	if err := plugin.Client.List(ctx, deploymentList, ctrlclient.InNamespace(plugin.Namespace)); err != nil {
		return appsv1.Deployment{},
			fmt.Errorf("could not get operator in namespace '%s': %w", plugin.Namespace, err)
	}

	var candidates []appsv1.Deployment
	for _, deployment := range deploymentList.Items {
		if isOperatorDeployment(deployment) {
			candidates = append(candidates, deployment)
		}
	}
	switch len(candidates) {
	case 1:
		return candidates[0], nil
	case 0:
		return appsv1.Deployment{}, errNoOperatorDeployment
	}
	names := make([]string, 0, len(candidates))
	for _, deployment := range candidates {
		names = append(names, deployment.Name)
	}
	slices.Sort(names)
	return appsv1.Deployment{},
		fmt.Errorf("several operator deployments in namespace '%s': %s\n"+
			"HINT: choose one with the '--deployment-name' option",
			plugin.Namespace, strings.Join(names, ", "))
}

// isOperatorDeployment is true for a Deployment carrying the operator label,
// or whose manager container runs the operator controller
func isOperatorDeployment(deployment appsv1.Deployment) bool {
	if deployment.Labels[labelOperatorNameKey] == labelOperatorName {
		return true
	}
	container := getManagerContainer(deployment)
	return container != nil &&
		slices.Contains(container.Command, managerCommand) &&
		slices.Contains(container.Args, managerControllerArg)
}

// getOperatorPods returns the pods of the operator Deployment, selected the
// way the Deployment selects them
func getOperatorPods(ctx context.Context, deployment appsv1.Deployment) ([]corev1.Pod, error) {
	if deployment.Spec.Selector == nil ||
		len(deployment.Spec.Selector.MatchLabels)+len(deployment.Spec.Selector.MatchExpressions) == 0 {
		return nil, errors.New("the deployment selects no pod")
	}
	selector, err := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
	if err != nil {
		return nil, fmt.Errorf("invalid deployment selector: %w", err)
	}

	podList := &corev1.PodList{}
	if err := plugin.Client.List(
		ctx, podList,
		ctrlclient.MatchingLabelsSelector{Selector: selector},
		ctrlclient.InNamespace(deployment.Namespace)); err != nil {
		return nil, err
	}

	if len(podList.Items) > 0 {
		return podList.Items, nil
	}

	return nil, fmt.Errorf("operator pods not found")
}

// operatorConfiguration reads the configuration of the operator the way the
// operator reads it, from the environment of its manager container, so that
// the names of its own objects resolve with the same defaults and derivations.
// A value coming from a reference, such as the namespace from the Downward
// API, is the one the Deployment implies; values set through envFrom or the
// configuration objects are not seen.
func operatorConfiguration(deployment appsv1.Deployment) *configuration.Data {
	data := map[string]string{}
	if container := getManagerContainer(deployment); container != nil {
		for _, env := range container.Env {
			if env.Value != "" {
				data[env.Name] = env.Value
			}
		}
	}
	data["OPERATOR_NAMESPACE"] = deployment.Namespace
	return configuration.NewFromData(data)
}

// webhookSecretName returns the name of the Secret holding the webhook
// serving certificate: the one mounted at the certificate directory of the
// manager container, or the one the configuration resolves
func webhookSecretName(deployment appsv1.Deployment, conf *configuration.Data) string {
	certDir := conf.WebhookCertDir
	if certDir == "" {
		certDir = certs.DefaultWebhookCertDir
	}
	if name := certs.MountedSecretName(&deployment, certDir); name != "" {
		return name
	}
	return conf.GetWebhookSecretName()
}

// getOperatorSecrets returns the secrets used by the operator: the serving
// certificate, the CA and the configuration Secret, under the names the
// operator resolves from its Deployment, plus the Secrets owned by the
// Deployment when they can be listed
func getOperatorSecrets(ctx context.Context, deployment appsv1.Deployment) ([]corev1.Secret, error) {
	contextLogger := log.FromContext(ctx)
	operatorNamespace := deployment.GetNamespace()
	conf := operatorConfiguration(deployment)

	secretNames := []string{
		webhookSecretName(deployment, conf),
		conf.GetCaSecretName(),
	}

	// get the operator config secrets name from deployment
	configSecretName, err := getOperatorConfigSecretName(deployment)
	if err != nil {
		return nil, err
	}
	if configSecretName != "" {
		secretNames = append(secretNames, configSecretName)
	} else {
		secretNames = append(secretNames, defaultConfigSecretName)
	}

	secrets := make([]corev1.Secret, 0, len(secretNames))
	for _, ss := range secretNames {
		var secret corev1.Secret
		err := plugin.Client.Get(ctx, types.NamespacedName{Name: ss, Namespace: operatorNamespace}, &secret)
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			contextLogger.Warning("could not get secret")
			return nil, err
		}
		secrets = append(secrets, secret)
	}

	// The Secrets the operator created for itself are owned by its Deployment,
	// whatever their names; listing them needs a right the names do not
	for _, secret := range getSecretsOwnedBy(ctx, deployment) {
		if !slices.ContainsFunc(secrets, func(s corev1.Secret) bool { return s.Name == secret.Name }) {
			secrets = append(secrets, secret)
		}
	}

	return secrets, nil
}

// getSecretsOwnedBy returns the Secrets of the namespace owned by the
// Deployment, or nothing when they cannot be listed
func getSecretsOwnedBy(ctx context.Context, deployment appsv1.Deployment) []corev1.Secret {
	secretList := &corev1.SecretList{}
	if err := plugin.Client.List(ctx, secretList, ctrlclient.InNamespace(deployment.Namespace)); err != nil {
		return nil
	}
	var owned []corev1.Secret
	for _, secret := range secretList.Items {
		for _, owner := range secret.OwnerReferences {
			if owner.UID == deployment.UID && owner.Kind == "Deployment" {
				owned = append(owned, secret)
				break
			}
		}
	}
	return owned
}

// getOperatorConfigMaps returns the configmap referenced by the operator
func getOperatorConfigMaps(ctx context.Context, deployment appsv1.Deployment) ([]corev1.ConfigMap, error) {
	contextLogger := log.FromContext(ctx)
	var configMaps []string
	operatorNamespace := deployment.GetNamespace()

	// get the operator configmap name from deployment
	configMapName, err := getOperatorConfigMapName(deployment)
	if err != nil {
		return nil, err
	}
	if configMapName != "" {
		configMaps = append(configMaps, configMapName)
	} else {
		configMaps = append(configMaps, defaultConfigmapName)
	}

	configs := make([]corev1.ConfigMap, 0, len(configMaps))
	for _, cm := range configMaps {
		var config corev1.ConfigMap
		err := plugin.Client.Get(ctx, types.NamespacedName{Name: cm, Namespace: operatorNamespace}, &config)
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			contextLogger.Warning("could not get configmap")
			return nil, err
		}
		configs = append(configs, config)
	}

	return configs, nil
}

// getOperatorConfigMapName return the name of configmap for operator configuration
func getOperatorConfigMapName(deployment appsv1.Deployment) (string, error) {
	return getManagerArg(deployment, "--config-map-name=")
}

// getOperatorConfigSecretName return the name of secret for operator configuration
func getOperatorConfigSecretName(deployment appsv1.Deployment) (string, error) {
	return getManagerArg(deployment, "--secret-name=")
}

// getManagerArg returns the value of an argument of the manager container,
// or an empty string when the argument is not given
func getManagerArg(deployment appsv1.Deployment, prefix string) (string, error) {
	container := getManagerContainer(deployment)
	if container == nil {
		return "", fmt.Errorf("can not find manager container from deployment")
	}
	for _, arg := range container.Args {
		if strings.HasPrefix(arg, prefix) {
			return strings.TrimPrefix(arg, prefix), nil
		}
	}
	return "", nil
}

// getManagerContainer get the running container from the deployment spec
func getManagerContainer(deployment appsv1.Deployment) *corev1.Container {
	for _, c := range deployment.Spec.Template.Spec.Containers {
		if c.Name == managerContainerName {
			return &c
		}
	}
	return nil
}
