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

package v1

import (
	"encoding/json"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/internal/scheme"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Cluster scale validation", func() {
	// A Cluster of three instances requiring two synchronous standbys: the
	// webhook of the Cluster refuses to bring it down to two instances
	newCluster := func() *apiv1.Cluster {
		cluster := &apiv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-example", Namespace: "default"},
			Spec: apiv1.ClusterSpec{
				Instances: 3,
				PostgresConfiguration: apiv1.PostgresConfiguration{
					Synchronous: &apiv1.SynchronousReplicaConfiguration{
						Method:         apiv1.SynchronousReplicaConfigurationMethodAny,
						Number:         2,
						DataDurability: apiv1.DataDurabilityLevelRequired,
					},
				},
				StorageConfiguration: apiv1.StorageConfiguration{Size: "1Gi"},
			},
		}
		cluster.Default()
		return cluster
	}

	newValidator := func(objects ...*apiv1.Cluster) *ClusterScaleValidator {
		builder := fake.NewClientBuilder().WithScheme(scheme.BuildWithAllKnownScheme())
		for _, object := range objects {
			builder = builder.WithObjects(object)
		}
		return &ClusterScaleValidator{
			reader:    builder.Build(),
			decoder:   admission.NewDecoder(scheme.BuildWithAllKnownScheme()),
			validator: newBypassableValidator[*apiv1.Cluster](&ClusterCustomValidator{}),
		}
	}

	scaleRequest := func(operation admissionv1.Operation, replicas int32) admission.Request {
		raw, err := json.Marshal(&autoscalingv1.Scale{
			TypeMeta:   metav1.TypeMeta{APIVersion: "autoscaling/v1", Kind: "Scale"},
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-example", Namespace: "default"},
			Spec:       autoscalingv1.ScaleSpec{Replicas: replicas},
		})
		Expect(err).ToNot(HaveOccurred())
		return admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
			Operation:   operation,
			Name:        "cluster-example",
			Namespace:   "default",
			SubResource: "scale",
			Object:      runtime.RawExtension{Raw: raw},
		}}
	}

	It("refuses a scale that the webhook of the Cluster refuses on the spec", func(ctx SpecContext) {
		response := newValidator(newCluster()).Handle(ctx, scaleRequest(admissionv1.Update, 2))
		Expect(response.Allowed).To(BeFalse())
		Expect(response.Result.Code).To(BeEquivalentTo(http.StatusUnprocessableEntity))
		Expect(response.Result.Message).To(ContainSubstring("spec.postgresql.synchronous"))
	})

	It("allows a valid scale", func(ctx SpecContext) {
		response := newValidator(newCluster()).Handle(ctx, scaleRequest(admissionv1.Update, 4))
		Expect(response.Allowed).To(BeTrue())
	})

	It("allows a scale that keeps the number of instances", func(ctx SpecContext) {
		cluster := newCluster()
		cluster.Spec.Instances = 2
		response := newValidator(cluster).Handle(ctx, scaleRequest(admissionv1.Update, 2))
		Expect(response.Allowed).To(BeTrue())
	})

	It("allows any scale when the validation of the Cluster is disabled", func(ctx SpecContext) {
		cluster := newCluster()
		cluster.Annotations = map[string]string{utils.WebhookValidationAnnotationName: "disabled"}
		response := newValidator(cluster).Handle(ctx, scaleRequest(admissionv1.Update, 2))
		Expect(response.Allowed).To(BeTrue())
	})

	It("fails when the Cluster cannot be read", func(ctx SpecContext) {
		response := newValidator().Handle(ctx, scaleRequest(admissionv1.Update, 2))
		Expect(response.Allowed).To(BeFalse())
		Expect(response.Result.Code).To(BeEquivalentTo(http.StatusInternalServerError))
	})

	It("ignores the operations other than update", func(ctx SpecContext) {
		response := newValidator().Handle(ctx, scaleRequest(admissionv1.Create, 2))
		Expect(response.Allowed).To(BeTrue())
	})
})
