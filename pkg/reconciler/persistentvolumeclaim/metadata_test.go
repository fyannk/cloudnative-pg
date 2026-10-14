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
	"maps"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/internal/configuration"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("metadataReconciler", func() {
	Describe("newLabelReconciler", func() {
		Context("when a PVC is not up-to-date", func() {
			It("should update the PVC with the correct labels", func() {
				cluster := &apiv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-cluster",
					},
					Spec: apiv1.ClusterSpec{
						InheritedMetadata: &apiv1.EmbeddedObjectMetadata{Labels: map[string]string{
							"label1":                     "value1",
							"label2":                     "value2",
							utils.KubernetesAppLabelName: "my-custom-app",
						}},
					},
					Status: apiv1.ClusterStatus{
						InstanceNames: []string{"instance1", "instance2"},
					},
				}
				pvc := &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pvc1",
						Labels: map[string]string{
							utils.PvcRoleLabelName: string(utils.PVCRolePgData),
						},
					},
				}
				reconciler := newLabelReconciler(cluster)

				// Assert that the PVC is not up-to-date
				Expect(reconciler.isUpToDate(pvc)).To(BeFalse())

				// Update the PVC labels
				reconciler.update(pvc)

				// Assert that the PVC is up-to-date with the correct labels
				Expect(pvc.Labels).To(HaveLen(6))
				Expect(pvc.Labels).To(HaveKeyWithValue("label1", "value1"))
				Expect(pvc.Labels).To(HaveKeyWithValue("label2", "value2"))
				Expect(pvc.Labels).To(HaveKeyWithValue(utils.PvcRoleLabelName, string(utils.PVCRolePgData)))
				Expect(pvc.Labels).To(HaveKeyWithValue(utils.KubernetesAppLabelName, "my-custom-app"))
				// Expected common labels
				Expect(pvc.Labels).To(HaveKeyWithValue(utils.KubernetesAppManagedByLabelName, utils.ManagerName))
				Expect(pvc.Labels).To(HaveKeyWithValue(utils.KubernetesAppComponentLabelName, utils.DatabaseComponentName))

				Expect(reconciler.isUpToDate(pvc)).To(BeTrue())
			})
		})

		Context("when a PVC is up-to-date", func() {
			It("should not update the PVC labels", func() {
				cluster := &apiv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-cluster",
					},
					Spec: apiv1.ClusterSpec{
						InheritedMetadata: &apiv1.EmbeddedObjectMetadata{Labels: map[string]string{
							"label1": "value1",
							"label2": "value2",
						}},
					},
					Status: apiv1.ClusterStatus{
						InstanceNames: []string{"instance1", "instance2"},
					},
				}
				pvc := &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pvc1",
						Labels: map[string]string{
							utils.PvcRoleLabelName:      string(utils.PVCRolePgData),
							utils.InstanceNameLabelName: "instance1",
							"label1":                    "value1",
							"label2":                    "value2",
							// Common labels
							utils.KubernetesAppManagedByLabelName: utils.ManagerName,
							utils.KubernetesAppLabelName:          utils.AppName,
							utils.KubernetesAppComponentLabelName: utils.DatabaseComponentName,
						},
						Annotations: map[string]string{},
					},
				}
				reconciler := newLabelReconciler(cluster)

				// Assert that the PVC is up-to-date
				Expect(reconciler.isUpToDate(pvc)).To(BeTrue())

				// Update the PVC labels
				reconciler.update(pvc)

				// Assert that the PVC labels are unchanged
				Expect(pvc.Labels).To(HaveLen(7))
				Expect(pvc.Labels).To(HaveKeyWithValue(utils.PvcRoleLabelName, string(utils.PVCRolePgData)))
				Expect(pvc.Labels).To(HaveKeyWithValue(utils.InstanceNameLabelName, "instance1"))
				Expect(pvc.Labels).To(HaveKeyWithValue("label1", "value1"))
				Expect(pvc.Labels).To(HaveKeyWithValue("label2", "value2"))
				// Expected common labels
				Expect(pvc.Labels).To(HaveKeyWithValue(utils.KubernetesAppManagedByLabelName, utils.ManagerName))
				Expect(pvc.Labels).To(HaveKeyWithValue(utils.KubernetesAppLabelName, utils.AppName))
				Expect(pvc.Labels).To(HaveKeyWithValue(utils.KubernetesAppComponentLabelName, utils.DatabaseComponentName))
			})
		})

		Context("when a PVC was just built with inheritedMetadata overriding the common labels", func() {
			It("should consider it up-to-date and leave the labels untouched", func() {
				inherited := map[string]string{
					utils.KubernetesAppLabelName:          "my-custom-app",
					utils.KubernetesAppManagedByLabelName: "my-gitops-tool",
					utils.KubernetesAppComponentLabelName: "my-custom-component",
				}
				cluster := &apiv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-cluster",
					},
					Spec: apiv1.ClusterSpec{
						InheritedMetadata: &apiv1.EmbeddedObjectMetadata{Labels: inherited},
					},
					Status: apiv1.ClusterStatus{
						InstanceNames: []string{"test-cluster-1"},
					},
				}
				pvc, err := Build(cluster, &CreateConfiguration{
					Status:     StatusReady,
					NodeSerial: 1,
					Calculator: NewPgDataCalculator(),
					Storage:    apiv1.StorageConfiguration{Size: "1Gi"},
				})
				Expect(err).NotTo(HaveOccurred())
				for key, value := range inherited {
					Expect(pvc.Labels).To(HaveKeyWithValue(key, value))
				}

				reconciler := newLabelReconciler(cluster)
				Expect(reconciler.isUpToDate(pvc)).To(BeTrue())

				builtLabels := maps.Clone(pvc.Labels)
				reconciler.update(pvc)
				Expect(pvc.Labels).To(Equal(builtLabels))
			})
		})

		Context("when a PVC has a nil labels map", func() {
			It("should not panic and should set the common labels", func() {
				cluster := &apiv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-cluster",
					},
				}
				pvc := &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pvc1",
					},
				}
				reconciler := newLabelReconciler(cluster)

				Expect(func() { reconciler.update(pvc) }).NotTo(Panic())
				Expect(pvc.Labels).To(HaveKeyWithValue(utils.KubernetesAppManagedByLabelName, utils.ManagerName))
			})
		})
	})
})

// The PVCs are updated in place with the metadata of their kind:
// objectMetadata.persistentVolumeClaims and the DEFAULT_* and MANDATORY_* settings for
// `persistentVolumeClaims`, next to the labels the operator manages itself.
var _ = Describe("metadata of the PVCs, reconciled in place", func() {
	var (
		previous *configuration.Data
		cluster  *apiv1.Cluster
		pvc      *corev1.PersistentVolumeClaim
	)

	reconcile := func() {
		for _, reconciler := range []metadataReconciler{newAnnotationReconciler(cluster), newLabelReconciler(cluster)} {
			if !reconciler.isUpToDate(pvc) {
				reconciler.update(pvc)
			}
			Expect(reconciler.isUpToDate(pvc)).To(BeTrue(), reconciler.name)
		}
	}

	BeforeEach(func() {
		previous = configuration.Current
		configuration.Current = configuration.NewConfiguration()
		DeferCleanup(func() { configuration.Current = previous })

		cluster = &apiv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: "payments"},
			Spec: apiv1.ClusterSpec{
				InheritedMetadata: &apiv1.EmbeddedObjectMetadata{Labels: map[string]string{"team": "payments"}},
				MetadataPropagation: []apiv1.MetadataPropagationRule{
					{
						Targets:     []apiv1.MetadataPropagationTarget{"pgPods"},
						Annotations: map[string]string{"vault.hashicorp.com/agent-inject": "true"},
					},
					{
						Targets:     []apiv1.MetadataPropagationTarget{"persistentVolumeClaims"},
						Labels:      map[string]string{"velero.io/exclude-from-backup": "true"},
						Annotations: map[string]string{"resize.topolvm.io/storage_limit": "200Gi"},
					},
				},
			},
			Status: apiv1.ClusterStatus{InstanceNames: []string{"payments-1"}},
		}
		pvc = &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{
			Name: "payments-1",
			Labels: map[string]string{
				utils.PvcRoleLabelName: string(utils.PVCRolePgData),
				"third-party":          "kept",
			},
			Annotations: map[string]string{"pv.kubernetes.io/bind-completed": "yes"},
		}}
	})

	It("adds the PVCs' metadata and the operator's labels, nothing of the Pods", func() {
		reconcile()
		Expect(pvc.Labels).To(HaveKeyWithValue("team", "payments"))
		Expect(pvc.Labels).To(HaveKeyWithValue("velero.io/exclude-from-backup", "true"))
		Expect(pvc.Labels).To(HaveKeyWithValue("third-party", "kept"))
		Expect(pvc.Labels).To(HaveKeyWithValue(utils.PvcRoleLabelName, string(utils.PVCRolePgData)))
		Expect(pvc.Labels).To(HaveKeyWithValue(utils.KubernetesAppManagedByLabelName, utils.ManagerName))
		Expect(pvc.Annotations).To(HaveKeyWithValue("resize.topolvm.io/storage_limit", "200Gi"))
		Expect(pvc.Annotations).To(HaveKeyWithValue("pv.kubernetes.io/bind-completed", "yes"))
		Expect(pvc.Annotations).NotTo(HaveKey("vault.hashicorp.com/agent-inject"))
	})

	It("follows the settings, defaults below the Cluster's metadata and mandatory values above", func() {
		configuration.Current.DefaultMetadata = `[` +
			`{"targets": ["persistentVolumeClaims"], "labels": {"velero.io/exclude-from-backup": "false"}},` +
			`{"targets": ["all"], "labels": {"env": "prod"}}` +
			`]`
		configuration.Current.MandatoryMetadata = `[` +
			`{"targets": ["persistentVolumeClaims"], "labels": {"team": "storage"}}` +
			`]`
		Expect(configuration.Current.ParseDefaultAndMandatoryMetadata()).To(Succeed())
		reconcile()
		Expect(pvc.Labels).To(HaveKeyWithValue("velero.io/exclude-from-backup", "true"))
		Expect(pvc.Labels).To(HaveKeyWithValue("env", "prod"))
		Expect(pvc.Labels).To(HaveKeyWithValue("team", "storage"))
	})

	It("cannot take away the labels the operator manages on a PVC", func() {
		cluster.Spec.MetadataPropagation[1].Labels[utils.PvcRoleLabelName] = "fake"
		cluster.Spec.MetadataPropagation[1].Labels[utils.KubernetesAppManagedByLabelName] = "fake"
		reconcile()
		Expect(pvc.Labels).To(HaveKeyWithValue(utils.PvcRoleLabelName, string(utils.PVCRolePgData)))
		Expect(pvc.Labels).To(HaveKeyWithValue(utils.KubernetesAppManagedByLabelName, utils.ManagerName))
	})
})
