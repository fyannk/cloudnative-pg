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

package persistentvolumeclaim

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/internal/scheme"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The answers of the API server to a PVC expansion it refuses, as the PVC
// resize admission and the ResourceQuota admission word them
const (
	storageClassRefusal = "only dynamically provisioned pvc can be resized and " +
		"the storageclass that provisions the pvc must support resize"
	quotaRefusal = "exceeded quota: storage-cap, requested: requests.storage=1Gi, " +
		"used: 1Gi, limited: 1Gi"
)

func forbiddenPVC(name, message string) error {
	return apierrs.NewForbidden(
		schema.GroupResource{Resource: "persistentvolumeclaims"}, name, errors.New(message))
}

var _ = Describe("Refused volume expansion", func() {
	const clusterName = "cluster-expansion"

	var (
		ctx      context.Context
		cluster  *apiv1.Cluster
		pvc      corev1.PersistentVolumeClaim
		recorder *record.FakeRecorder
		// refusal is the answer to the next PVC patches, nil to accept them
		refusal error
		cli     client.Client
	)

	condition := func() *metav1.Condition {
		var stored apiv1.Cluster
		Expect(cli.Get(ctx, client.ObjectKeyFromObject(cluster), &stored)).To(Succeed())
		return meta.FindStatusCondition(stored.Status.Conditions, string(apiv1.ConditionVolumeExpansionRefused))
	}

	BeforeEach(func() {
		ctx = context.Background()
		cluster = &apiv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: clusterName, Namespace: "default", Generation: 3},
			Spec: apiv1.ClusterSpec{
				StorageConfiguration: apiv1.StorageConfiguration{Size: "2Gi"},
			},
		}
		pvc = makePVC(clusterName, "1", "1", NewPgDataCalculator(), false)
		pvc.Namespace = "default"
		pvc.Spec.StorageClassName = ptr.To("local-path")
		pvc.Spec.Resources.Requests = corev1.ResourceList{"storage": resource.MustParse("1Gi")}
		recorder = record.NewFakeRecorder(10)
		refusal = nil
		cli = fake.NewClientBuilder().
			WithScheme(scheme.BuildWithAllKnownScheme()).
			WithObjects(cluster, &pvc).
			WithStatusSubresource(cluster).
			WithInterceptorFuncs(interceptor.Funcs{
				Patch: func(ctx context.Context, c client.WithWatch, obj client.Object,
					patch client.Patch, opts ...client.PatchOption,
				) error {
					if _, isPVC := obj.(*corev1.PersistentVolumeClaim); isPVC && refusal != nil {
						return refusal
					}
					return c.Patch(ctx, obj, patch, opts...)
				},
			}).
			Build()
	})

	// The life of the condition over four reconciliations: refused, still
	// refused, accepted, still accepted
	It("reports the refusal on the cluster, once per change", func() {
		pvcs := func() []corev1.PersistentVolumeClaim {
			var list corev1.PersistentVolumeClaimList
			Expect(cli.List(ctx, &list)).To(Succeed())
			return list.Items
		}

		// The StorageClass forbids the expansion: the reconciliation still
		// fails, and the cluster now says why
		refusal = forbiddenPVC(pvc.Name, storageClassRefusal)
		_, err := Reconcile(ctx, cli, recorder, cluster, nil, pvcs())
		Expect(err).To(MatchError(ContainSubstring(storageClassRefusal)))
		Expect(apierrs.IsForbidden(err)).To(BeTrue())

		refused := condition()
		Expect(refused).ToNot(BeNil())
		Expect(refused.Status).To(Equal(metav1.ConditionTrue))
		Expect(refused.Reason).To(Equal(string(apiv1.ConditionReasonStorageClassForbidsExpansion)))
		Expect(refused.ObservedGeneration).To(BeEquivalentTo(3))
		Expect(refused.Message).To(And(
			ContainSubstring(`cluster-expansion-1 to 2Gi (StorageClass "local-path")`),
			ContainSubstring(storageClassRefusal),
			ContainSubstring("allowVolumeExpansion"),
			ContainSubstring("resizeInUseVolumes"),
		))
		Expect(recorder.Events).To(Receive(Equal(
			"Warning StorageClassForbidsExpansion " + refused.Message)))

		// The same refusal on the next pass: no new Event
		_, err = Reconcile(ctx, cli, recorder, cluster, nil, pvcs())
		Expect(err).To(HaveOccurred())
		Expect(condition().Message).To(Equal(refused.Message))
		Expect(recorder.Events).ToNot(Receive())

		// The StorageClass now allows it: the condition turns False, with
		// one Normal Event
		refusal = nil
		_, err = Reconcile(ctx, cli, recorder, cluster, nil, pvcs())
		Expect(err).ToNot(HaveOccurred())
		accepted := condition()
		Expect(accepted.Status).To(Equal(metav1.ConditionFalse))
		Expect(accepted.Reason).To(Equal(string(apiv1.ConditionReasonVolumeExpansionAccepted)))
		Expect(recorder.Events).To(Receive(HavePrefix("Normal VolumeExpansionAccepted ")))

		// Nothing more to say on the next clean pass
		_, err = Reconcile(ctx, cli, recorder, cluster, nil, pvcs())
		Expect(err).ToNot(HaveOccurred())
		Expect(condition().Status).To(Equal(metav1.ConditionFalse))
		Expect(recorder.Events).ToNot(Receive())
	})

	It("adds no condition to a cluster that never had a refusal", func() {
		_, err := Reconcile(ctx, cli, recorder, cluster, nil, []corev1.PersistentVolumeClaim{pvc})
		Expect(err).ToNot(HaveOccurred())
		Expect(condition()).To(BeNil())
		Expect(recorder.Events).ToNot(Receive())
	})

	It("reports nothing for an error that is not a refusal", func() {
		refusal = apierrs.NewInvalid(schema.GroupKind{Kind: "PersistentVolumeClaim"}, pvc.Name, nil)
		_, err := Reconcile(ctx, cli, recorder, cluster, nil, []corev1.PersistentVolumeClaim{pvc})
		Expect(err).To(MatchError(ContainSubstring("error while changing PVC storage requirement")))
		Expect(condition()).To(BeNil())
		Expect(recorder.Events).ToNot(Receive())
	})

	DescribeTable("classifies the answer of the API server",
		func(answer string, reason apiv1.ConditionReason, advice string) {
			refused := volumeExpansionRefusedCondition([]*volumeExpansionRefusal{
				newVolumeExpansionRefusal(&pvc, resource.MustParse("2Gi"), forbiddenPVC(pvc.Name, answer)),
			})
			Expect(refused.Reason).To(Equal(string(reason)))
			Expect(refused.Message).To(ContainSubstring(answer))
			Expect(refused.Message).To(ContainSubstring(advice))
		},
		Entry("StorageClass", storageClassRefusal,
			apiv1.ConditionReasonStorageClassForbidsExpansion, "allowVolumeExpansion"),
		Entry("ResourceQuota", quotaRefusal,
			apiv1.ConditionReasonQuotaExceeded, "ResourceQuota"),
		Entry("failed quota", "failed quota: storage-cap: must specify requests.storage",
			apiv1.ConditionReasonQuotaExceeded, "ResourceQuota"),
		Entry("admission webhook", `admission webhook "deny.example.com" denied the request`,
			apiv1.ConditionReasonExpansionForbidden, "Check what refused the request"),
	)

	It("lists the refused PVCs by name, at most ten", func() {
		var refusals []*volumeExpansionRefusal
		for idx := 12; idx >= 1; idx-- {
			refused := makePVC(clusterName, fmt.Sprintf("%02d", idx), "1", NewPgDataCalculator(), false)
			refusals = append(refusals, newVolumeExpansionRefusal(&refused, resource.MustParse("2Gi"),
				forbiddenPVC(refused.Name, storageClassRefusal)))
		}

		message := volumeExpansionRefusedCondition(refusals).Message
		Expect(message).To(HavePrefix("The API server refused to expand 12 PVC(s) to the size requested " +
			"by the cluster: cluster-expansion-01 to 2Gi (the default StorageClass), " +
			"cluster-expansion-02 to 2Gi"))
		Expect(message).To(ContainSubstring("cluster-expansion-10 to 2Gi (the default StorageClass) and 2 more."))
		Expect(message).ToNot(ContainSubstring("cluster-expansion-11"))
		Expect(message).To(ContainSubstring(`Answer for cluster-expansion-01: persistentvolumeclaims ` +
			`"cluster-expansion-01" is forbidden`))
	})
})
