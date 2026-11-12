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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func createFakeOperatorDeploymentByName(ctx context.Context,
	kubeClient client.Client,
	deploymentName string,
	labels map[string]string,
) error {
	operatorDep := appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: operatorNamespaceName,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{},
	}

	return kubeClient.Create(ctx, &operatorDep)
}

func deleteFakeOperatorDeployment(ctx context.Context,
	kubeClient client.Client,
	deploymentName string,
) error {
	operatorDep := appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      deploymentName,
			Namespace: operatorNamespaceName,
		},
		Spec: appsv1.DeploymentSpec{},
	}

	return kubeClient.Delete(ctx, &operatorDep)
}

var _ = Describe("Difference of values of maps", func() {
	It("will always set the app.kubernetes.io/name to cloudnative-pg", func(ctx SpecContext) {
		operatorLabelSelector := "app.kubernetes.io/name=cloudnative-pg"
		operatorLabels := map[string]string{
			"app.kubernetes.io/name": "cloudnative-pg",
		}
		kubeClient := generateFakeClient()
		err := createFakeOperatorDeploymentByName(ctx, kubeClient, operatorDeploymentName, operatorLabels)
		Expect(err).ToNot(HaveOccurred())
		labelMap, err := labels.ConvertSelectorToLabelsMap(operatorLabelSelector)
		Expect(err).ToNot(HaveOccurred())

		deployment, err := findOperatorDeploymentByFilter(ctx,
			kubeClient,
			operatorNamespaceName,
			client.MatchingLabelsSelector{Selector: labelMap.AsSelector()})
		Expect(err).ToNot(HaveOccurred())
		Expect(deployment).ToNot(BeNil())

		err = deleteFakeOperatorDeployment(ctx, kubeClient, operatorDeploymentName)
		Expect(err).ToNot(HaveOccurred())

		operatorLabels = map[string]string{
			"app.kubernetes.io/name": "some-app",
		}
		err = createFakeOperatorDeploymentByName(ctx, kubeClient, "some-app", operatorLabels)
		Expect(err).ToNot(HaveOccurred())
		deployment, err = findOperatorDeploymentByFilter(ctx,
			kubeClient,
			operatorNamespaceName,
			client.MatchingLabelsSelector{Selector: labelMap.AsSelector()})
		Expect(err).To(HaveOccurred())
		Expect(deployment).To(BeNil())

		operatorLabels = map[string]string{
			"app.kubernetes.io/name": "cloudnative-pg",
		}
		err = createFakeOperatorDeploymentByName(ctx, kubeClient, operatorNamespaceName, operatorLabels)
		Expect(err).ToNot(HaveOccurred())
		deployment, err = findOperatorDeploymentByFilter(ctx,
			kubeClient,
			operatorNamespaceName,
			client.MatchingLabelsSelector{Selector: labelMap.AsSelector()})
		Expect(err).ToNot(HaveOccurred())
		Expect(deployment).ToNot(BeNil())
	})
})

var _ = Describe("Operator deployment lookup by name", func() {
	operatorLabelSelector := "app.kubernetes.io/name=cloudnative-pg"
	operatorLabels := map[string]string{"app.kubernetes.io/name": "cloudnative-pg"}

	It("returns the deployment named, whatever its labels", func(ctx SpecContext) {
		kubeClient := generateFakeClient()
		Expect(createFakeOperatorDeploymentByName(ctx, kubeClient, "cnpg-toto",
			map[string]string{"app.kubernetes.io/name": "toto"})).To(Succeed())
		Expect(createFakeOperatorDeploymentByName(ctx, kubeClient, "cnpg-tata",
			map[string]string{"app.kubernetes.io/name": "tata"})).To(Succeed())

		deployment, err := GetOperatorDeployment(ctx, kubeClient, operatorNamespaceName, "cnpg-toto",
			operatorLabelSelector)
		Expect(err).ToNot(HaveOccurred())
		Expect(deployment.Name).To(Equal("cnpg-toto"))
	})

	It("prefers the name over the label when several deployments carry the label", func(ctx SpecContext) {
		kubeClient := generateFakeClient()
		Expect(createFakeOperatorDeploymentByName(ctx, kubeClient, "cnpg-toto", operatorLabels)).To(Succeed())
		Expect(createFakeOperatorDeploymentByName(ctx, kubeClient, "cnpg-tata", operatorLabels)).To(Succeed())

		deployment, err := GetOperatorDeployment(ctx, kubeClient, operatorNamespaceName, "cnpg-tata",
			operatorLabelSelector)
		Expect(err).ToNot(HaveOccurred())
		Expect(deployment.Name).To(Equal("cnpg-tata"))

		_, err = GetOperatorDeployment(ctx, kubeClient, operatorNamespaceName, "", operatorLabelSelector)
		Expect(err).To(MatchError(ContainSubstring("more than one operator deployment")))
	})

	It("explains a missing deployment of the name given, keeping the not-found nature", func(ctx SpecContext) {
		kubeClient := generateFakeClient()
		Expect(createFakeOperatorDeploymentByName(ctx, kubeClient, operatorDeploymentName, operatorLabels)).
			To(Succeed())

		_, err := GetOperatorDeployment(ctx, kubeClient, operatorNamespaceName, "cnpg-toto", operatorLabelSelector)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(`operator deployment "cnpg-toto" not found in namespace`))
		Expect(err.Error()).To(ContainSubstring("OPERATOR_DEPLOYMENT_NAME"))
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("searches by label when no name is given", func(ctx SpecContext) {
		kubeClient := generateFakeClient()
		Expect(createFakeOperatorDeploymentByName(ctx, kubeClient, operatorDeploymentName, operatorLabels)).
			To(Succeed())

		deployment, err := GetOperatorDeployment(ctx, kubeClient, operatorNamespaceName, "", operatorLabelSelector)
		Expect(err).ToNot(HaveOccurred())
		Expect(deployment.Name).To(Equal(operatorDeploymentName))
	})

	It("sets the owner from the deployment named", func(ctx SpecContext) {
		kubeClient := generateFakeClient()
		Expect(createFakeOperatorDeploymentByName(ctx, kubeClient, "cnpg-toto",
			map[string]string{"app.kubernetes.io/name": "toto"})).To(Succeed())

		controlled := metav1.ObjectMeta{Name: "cnpg-toto-ca-secret", Namespace: operatorNamespaceName}
		Expect(SetAsOwnedByOperatorDeployment(ctx, kubeClient, &controlled, "cnpg-toto", operatorLabelSelector)).
			To(Succeed())
		Expect(controlled.OwnerReferences).To(HaveLen(1))
		Expect(controlled.OwnerReferences[0].Name).To(Equal("cnpg-toto"))
		Expect(controlled.OwnerReferences[0].Kind).To(Equal("Deployment"))
	})
})
