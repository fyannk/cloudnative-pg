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
	"fmt"
	"slices"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/internal/cmd/plugin"
)

// getWebhooks returns the webhook configurations of the operator: the two it
// manages, fetched by the names it resolves from its Deployment, plus, when
// the cluster-scoped list is allowed, the ones of the postgresql.cnpg.io group
// pointing at a Service of the operator namespace that selects the Pods of
// the Deployment. Their client configuration is redacted unless stopRedact is
// set.
func getWebhooks(
	ctx context.Context,
	deployment appsv1.Deployment,
	stopRedact bool,
) (
	*admissionregistrationv1.MutatingWebhookConfigurationList,
	*admissionregistrationv1.ValidatingWebhookConfigurationList,
	error,
) {
	conf := operatorConfiguration(deployment)
	mWebhookConfig := admissionregistrationv1.MutatingWebhookConfigurationList{}
	vWebhookConfig := admissionregistrationv1.ValidatingWebhookConfigurationList{}

	var mutating admissionregistrationv1.MutatingWebhookConfiguration
	if err := plugin.Client.Get(ctx,
		types.NamespacedName{Name: conf.GetMutatingWebhookConfigurationName()}, &mutating); err == nil {
		mWebhookConfig.Items = append(mWebhookConfig.Items, mutating)
	}
	var validating admissionregistrationv1.ValidatingWebhookConfiguration
	if err := plugin.Client.Get(ctx,
		types.NamespacedName{Name: conf.GetValidatingWebhookConfigurationName()}, &validating); err == nil {
		vWebhookConfig.Items = append(vWebhookConfig.Items, validating)
	}

	listErr := listWebhooksOfDeployment(ctx, deployment, &mWebhookConfig, &vWebhookConfig)

	if !stopRedact {
		redactWebhooks(&mWebhookConfig, &vWebhookConfig)
	}

	if len(mWebhookConfig.Items) == 0 && len(vWebhookConfig.Items) == 0 {
		if listErr != nil {
			return nil, nil, listErr
		}
		return nil, nil, fmt.Errorf(
			"can't find the webhooks that targeting resources within the group %s",
			apiv1.SchemeGroupVersion.Group,
		)
	}

	return &mWebhookConfig, &vWebhookConfig, nil
}

// listWebhooksOfDeployment adds to the lists the webhook configurations of the
// postgresql.cnpg.io group pointing at the Deployment and not already there.
// The first list that is not allowed is returned as an error, the other list
// is still tried.
func listWebhooksOfDeployment(
	ctx context.Context,
	deployment appsv1.Deployment,
	mWebhookConfig *admissionregistrationv1.MutatingWebhookConfigurationList,
	vWebhookConfig *admissionregistrationv1.ValidatingWebhookConfigurationList,
) error {
	var listErr error

	var mutatingList admissionregistrationv1.MutatingWebhookConfigurationList
	if err := plugin.Client.List(ctx, &mutatingList); err != nil {
		listErr = fmt.Errorf("insufficient permissions to list mutating webhooks: %w", err)
	}
	for _, item := range mutatingList.Items {
		clientConfigs := make([]admissionregistrationv1.WebhookClientConfig, 0, len(item.Webhooks))
		for _, webhook := range item.Webhooks {
			if isWebhookOfGroup(webhook.Rules) {
				clientConfigs = append(clientConfigs, webhook.ClientConfig)
			}
		}
		if pointsAtDeployment(ctx, clientConfigs, deployment) &&
			!slices.ContainsFunc(mWebhookConfig.Items,
				func(m admissionregistrationv1.MutatingWebhookConfiguration) bool { return m.Name == item.Name }) {
			mWebhookConfig.Items = append(mWebhookConfig.Items, item)
		}
	}

	var validatingList admissionregistrationv1.ValidatingWebhookConfigurationList
	if err := plugin.Client.List(ctx, &validatingList); err != nil && listErr == nil {
		listErr = fmt.Errorf("insufficient permissions to list validating webhooks: %w", err)
	}
	for _, item := range validatingList.Items {
		clientConfigs := make([]admissionregistrationv1.WebhookClientConfig, 0, len(item.Webhooks))
		for _, webhook := range item.Webhooks {
			if isWebhookOfGroup(webhook.Rules) {
				clientConfigs = append(clientConfigs, webhook.ClientConfig)
			}
		}
		if pointsAtDeployment(ctx, clientConfigs, deployment) &&
			!slices.ContainsFunc(vWebhookConfig.Items,
				func(v admissionregistrationv1.ValidatingWebhookConfiguration) bool { return v.Name == item.Name }) {
			vWebhookConfig.Items = append(vWebhookConfig.Items, item)
		}
	}

	return listErr
}

// isWebhookOfGroup is true when the webhook rules target the postgresql.cnpg.io group
func isWebhookOfGroup(rules []admissionregistrationv1.RuleWithOperations) bool {
	return len(rules) > 0 && len(rules[0].APIGroups) > 0 && rules[0].APIGroups[0] == apiv1.SchemeGroupVersion.Group
}

// redactWebhooks redacts the client configuration of every webhook of the lists
func redactWebhooks(
	mWebhookConfig *admissionregistrationv1.MutatingWebhookConfigurationList,
	vWebhookConfig *admissionregistrationv1.ValidatingWebhookConfigurationList,
) {
	for i, item := range mWebhookConfig.Items {
		for j, webhook := range item.Webhooks {
			mWebhookConfig.Items[i].Webhooks[j].ClientConfig = redactWebhookClientConfig(webhook.ClientConfig)
		}
	}
	for i, item := range vWebhookConfig.Items {
		for j, webhook := range item.Webhooks {
			vWebhookConfig.Items[i].Webhooks[j].ClientConfig = redactWebhookClientConfig(webhook.ClientConfig)
		}
	}
}

// pointsAtDeployment is true when at least one client configuration points at
// a Service of the Deployment namespace selecting its Pods. A Service that
// cannot be read counts as selecting them: the namespace is then the only
// evidence.
func pointsAtDeployment(
	ctx context.Context,
	configs []admissionregistrationv1.WebhookClientConfig,
	deployment appsv1.Deployment,
) bool {
	for _, config := range configs {
		if config.Service == nil || config.Service.Namespace != deployment.Namespace {
			continue
		}
		var service corev1.Service
		if err := plugin.Client.Get(ctx,
			types.NamespacedName{Name: config.Service.Name, Namespace: config.Service.Namespace}, &service); err != nil {
			return true
		}
		if selectsPods(service.Spec.Selector, deployment.Spec.Template.Labels) {
			return true
		}
	}
	return false
}

// selectsPods is true when the selector is not empty and every one of its
// labels is carried by the pod labels
func selectsPods(selector, podLabels map[string]string) bool {
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

// getWebhookServices returns the Services the webhook configurations point
// at, each one once
func getWebhookServices(
	ctx context.Context,
	mutatingWebhookList *admissionregistrationv1.MutatingWebhookConfigurationList,
	validatingWebhookList *admissionregistrationv1.ValidatingWebhookConfigurationList,
) ([]corev1.Service, error) {
	var references []types.NamespacedName
	addReference := func(config admissionregistrationv1.WebhookClientConfig) {
		if config.Service == nil {
			return
		}
		key := types.NamespacedName{Name: config.Service.Name, Namespace: config.Service.Namespace}
		if !slices.Contains(references, key) {
			references = append(references, key)
		}
	}
	if mutatingWebhookList != nil {
		for _, item := range mutatingWebhookList.Items {
			for _, webhook := range item.Webhooks {
				addReference(webhook.ClientConfig)
			}
		}
	}
	if validatingWebhookList != nil {
		for _, item := range validatingWebhookList.Items {
			for _, webhook := range item.Webhooks {
				addReference(webhook.ClientConfig)
			}
		}
	}

	services := make([]corev1.Service, 0, len(references))
	for _, key := range references {
		var service corev1.Service
		if err := plugin.Client.Get(ctx, key, &service); err != nil {
			return services, err
		}
		services = append(services, service)
	}
	return services, nil
}
