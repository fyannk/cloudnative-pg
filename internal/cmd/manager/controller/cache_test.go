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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/cloudnative-pg/cloudnative-pg/internal/configuration"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Manager cache options", func() {
	It("stores objects without their managedFields and keeps the per-kind filters", func() {
		opts := cacheOptions(&configuration.Data{OperatorNamespace: "cnpg-system"})

		// A Secret as the API server lists it: one managedFields entry per
		// writer, each one often bigger than the Secret's own data.
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app",
				Namespace: "tenant",
				Labels:    map[string]string{"cnpg.io/cluster": "cluster-example"},
				ManagedFields: []metav1.ManagedFieldsEntry{
					{Manager: "manager", Operation: metav1.ManagedFieldsOperationApply},
				},
			},
			Data: map[string][]byte{"password": []byte("secret")},
		}

		Expect(opts.DefaultTransform).NotTo(BeNil())
		stored, err := opts.DefaultTransform(secret)
		Expect(err).NotTo(HaveOccurred())
		Expect(stored.(*corev1.Secret).ManagedFields).To(BeNil())
		// Everything the controllers read is untouched.
		Expect(stored.(*corev1.Secret).Labels).To(HaveKeyWithValue("cnpg.io/cluster", "cluster-example"))
		Expect(stored.(*corev1.Secret).Data).To(HaveKeyWithValue("password", []byte("secret")))

		// The existing filters are still there: Helm release Secrets are
		// not listed, EndpointSlices only in the operator namespace.
		Expect(opts.ByObject).To(HaveLen(2))
		for obj, byObject := range opts.ByObject {
			if _, isSecret := obj.(*corev1.Secret); isSecret {
				Expect(byObject.Field.String()).To(Equal("type!=helm.sh/release.v1"))
			}
		}
	})
})
