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

	"github.com/cloudnative-pg/machinery/pkg/log"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"
)

// SetAsOwnedByOperatorDeployment sets the controlled object as owned by the operator deployment.
//
// IMPORTANT: The controlled resource must reside in the same namespace as the operator as described by:
// https://kubernetes.io/docs/concepts/overview/working-with-objects/owners-dependents/
func SetAsOwnedByOperatorDeployment(ctx context.Context,
	kubeClient client.Client,
	controlled *metav1.ObjectMeta,
	operatorDeploymentName string,
	operatorLabelSelector string,
) error {
	deployment, err := GetOperatorDeployment(ctx, kubeClient, controlled.Namespace,
		operatorDeploymentName, operatorLabelSelector)
	if err != nil {
		return err
	}

	setAsOwnedByDeployment(controlled, deployment)
	return nil
}

// setAsOwnedByDeployment sets the controlled object as owned by the given deployment
func setAsOwnedByDeployment(controlled *metav1.ObjectMeta, deployment *appsv1.Deployment) {
	// The deployment typeMeta is empty (kubernetes bug), so we need to explicitly populate it.
	typeMeta := metav1.TypeMeta{
		Kind:       "Deployment",
		APIVersion: "apps/v1",
	}
	utils.SetAsOwnedBy(controlled, deployment.ObjectMeta, typeMeta)
}

// GetOperatorDeployment returns the operator deployment. When its name is
// known, the deployment is fetched by name and a missing one is an error
// explaining where the name comes from. Otherwise the deployment is searched
// by label, then by the OLM label; no match or more than one match is an
// error.
func GetOperatorDeployment(
	ctx context.Context,
	kubeClient client.Client,
	namespace, operatorDeploymentName, operatorLabelSelector string,
) (*appsv1.Deployment, error) {
	if operatorDeploymentName != "" {
		deployment := &appsv1.Deployment{}
		err := kubeClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: operatorDeploymentName}, deployment)
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf(
				"operator deployment %q not found in namespace %q: "+
					"OPERATOR_DEPLOYMENT_NAME must be the name of the Deployment running the operator: %w",
				operatorDeploymentName, namespace, err)
		}
		if err != nil {
			return nil, err
		}
		return deployment, nil
	}

	labelMap, err := labels.ConvertSelectorToLabelsMap(operatorLabelSelector)
	if err != nil {
		return nil, err
	}
	deployment, err := findOperatorDeploymentByFilter(ctx,
		kubeClient,
		namespace,
		client.MatchingLabelsSelector{Selector: labelMap.AsSelector()})
	if err != nil {
		return nil, err
	}
	if deployment != nil {
		return deployment, nil
	}

	deployment, err = findOperatorDeploymentByFilter(ctx,
		kubeClient,
		namespace,
		client.HasLabels{"operators.coreos.com/cloudnative-pg.openshift-operators="})
	if err != nil {
		return nil, err
	}
	if deployment != nil {
		return deployment, nil
	}

	return nil, fmt.Errorf("no deployment detected")
}

// findOperatorDeploymentByFilter search in a defined namespace
// looking for a deployment with the defined filter
func findOperatorDeploymentByFilter(ctx context.Context,
	kubeClient client.Client,
	namespace string,
	filter client.ListOption,
) (*appsv1.Deployment, error) {
	logger := log.FromContext(ctx)

	deploymentList := &appsv1.DeploymentList{}
	err := kubeClient.List(
		ctx,
		deploymentList,
		client.InNamespace(namespace),
		filter,
	)
	if err != nil {
		return nil, err
	}
	switch {
	case len(deploymentList.Items) == 1:
		return &deploymentList.Items[0], nil
	case len(deploymentList.Items) > 1:
		err = fmt.Errorf("more than one operator deployment running")
		logger.Error(err, "more than one operator deployment found with the filter", "filter", filter)
		return nil, err
	}

	err = fmt.Errorf("no operator deployment found")
	logger.Error(err, "no operator deployment found with the filter", "filter", filter)
	return nil, err
}
