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

package operatormetrics

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type fixedMetadataCounter map[string]int

func (f fixedMetadataCounter) CachedObjects(context.Context) (map[string]int, error) {
	return f, nil
}

// One scrape of a cache holding objects the operator manages and user
// objects: each kind is split on the cnpg.io/reconcile label, a kind without
// objects reports zeros, and the reload watch reports its metadata per kind.
// Without the filter, the managed="false" series are what enabling it removes.
var _ = Describe("The operator cache metrics", func() {
	It("splits each kind on the managed label and reports the metadata watch", func() {
		managed := map[string]string{utils.ReconcileLabelName: "true"}
		reader := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(
			&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "a", Name: "ca", Labels: managed}},
			&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "a", Name: "app", Labels: managed}},
			&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: "a", Name: "helm-release"}},
			&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "a", Name: "pg-1", Labels: managed}},
		).Build()

		series := gather(NewCacheCollector(reader, scheme.Scheme,
			[]client.Object{&corev1.Secret{}, &corev1.Pod{}, &corev1.ConfigMap{}},
			fixedMetadataCounter{"Secret": 3, "ConfigMap": 1}))

		Expect(series).To(Equal(map[string]float64{
			`cnpg_operator_cached_objects{kind="Secret",managed="true"}`:     2,
			`cnpg_operator_cached_objects{kind="Secret",managed="false"}`:    1,
			`cnpg_operator_cached_objects{kind="Pod",managed="true"}`:        1,
			`cnpg_operator_cached_objects{kind="Pod",managed="false"}`:       0,
			`cnpg_operator_cached_objects{kind="ConfigMap",managed="true"}`:  0,
			`cnpg_operator_cached_objects{kind="ConfigMap",managed="false"}`: 0,
			`cnpg_operator_metadata_cached_objects{kind="Secret"}`:           3,
			`cnpg_operator_metadata_cached_objects{kind="ConfigMap"}`:        1,
		}))
	})
})
