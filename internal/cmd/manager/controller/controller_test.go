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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/cloudnative-pg/cloudnative-pg/internal/configuration"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Reading the configuration ConfigMap sets every field again from the
// environment, the data or the defaults: without OPERATOR_NAMESPACE, it
// dropped the namespace found from the service account
var _ = Describe("loadConfiguration", func() {
	It("keeps the namespace found from the service account", func() {
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "cnpg-controller-manager-config", Namespace: "cnpg-system"},
			Data:       map[string]string{"INHERITED_LABELS": "team"},
		}
		kubeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(configMap).Build()
		conf := configuration.NewConfiguration()
		conf.OperatorNamespace = "cnpg-system"

		Expect(loadConfiguration(context.Background(), kubeClient, configMap.Name, "", conf)).To(Succeed())

		Expect(conf.InheritedLabels).To(ConsistOf("team"))
		Expect(conf.OperatorNamespace).To(Equal("cnpg-system"))
	})
})
