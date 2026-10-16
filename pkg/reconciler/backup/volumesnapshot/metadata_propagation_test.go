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

package volumesnapshot

import (
	"context"
	"errors"

	volumesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/internal/configuration"
	"github.com/cloudnative-pg/cloudnative-pg/internal/scheme"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/management/postgres/webserver/client/remote"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// unreachableInstance answers no pg_controldata, as an instance that cannot
// be reached: the snapshot is created without it
type unreachableInstance struct {
	remote.InstanceClient
}

func (unreachableInstance) GetPgControlDataFromInstance(context.Context, *corev1.Pod) (string, error) {
	return "", errors.New("unreachable")
}

// The labels and annotations of a VolumeSnapshot come, by increasing
// precedence, from the default metadata of the operator, its PVC, the
// volume snapshot settings of the Cluster and, when the Cluster owns it, the
// metadata of the Cluster, then the rules of the Backup, then the mandatory
// metadata of the operator. The keys the operator sets itself win over all
var _ = Describe("The metadata of the VolumeSnapshots of a Backup", func() {
	const namespace = "payments"

	var (
		cluster *apiv1.Cluster
		backup  *apiv1.Backup
		pvc     *corev1.PersistentVolumeClaim
	)

	settings := func(defaults, mandatory string) {
		configuration.Current.DefaultMetadata = defaults
		configuration.Current.MandatoryMetadata = mandatory
		Expect(configuration.Current.ParseDefaultAndMandatoryMetadata()).To(Succeed())
	}

	BeforeEach(func() {
		previous := configuration.Current
		configuration.Current = configuration.NewConfiguration()
		DeferCleanup(func() { configuration.Current = previous })

		cluster = &apiv1.Cluster{
			TypeMeta:   metav1.TypeMeta{Kind: apiv1.ClusterKind, APIVersion: apiv1.SchemeGroupVersion.String()},
			ObjectMeta: metav1.ObjectMeta{Name: "payments", Namespace: namespace},
			Spec: apiv1.ClusterSpec{
				InheritedMetadata: &apiv1.EmbeddedObjectMetadata{
					Labels: map[string]string{"inherited": "cluster", "shared": "inherited"},
				},
				Backup: &apiv1.BackupConfiguration{VolumeSnapshot: &apiv1.VolumeSnapshotConfiguration{
					ClassName: "csi-hostpath-snapclass",
					Online:    ptr.To(false),
					Labels:    map[string]string{"config": "snapshot", "shared": "config", "pvc-and-config": "config"},
					Annotations: map[string]string{
						"config-note": "snapshot",
					},
				}},
			},
		}
		backup = &apiv1.Backup{
			TypeMeta:   metav1.TypeMeta{Kind: apiv1.BackupKind, APIVersion: apiv1.SchemeGroupVersion.String()},
			ObjectMeta: metav1.ObjectMeta{Name: "nightly", Namespace: namespace},
			Spec:       apiv1.BackupSpec{Method: apiv1.BackupMethodVolumeSnapshot},
			Status:     apiv1.BackupStatus{MajorVersion: 18},
		}
		pvc = &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{
			Name:      "payments-1",
			Namespace: namespace,
			Labels: map[string]string{
				utils.PvcRoleLabelName: string(utils.PVCRolePgData),
				"pvc":                  "data",
				"pvc-and-config":       "pvc",
			},
			Annotations: map[string]string{"pvc-note": "data"},
		}}
	})

	snapshot := func(ctx context.Context, owner apiv1.SnapshotOwnerReference) volumesnapshotv1.VolumeSnapshot {
		cluster.Spec.Backup.VolumeSnapshot.SnapshotOwnerReference = owner
		cli := fake.NewClientBuilder().WithScheme(scheme.BuildWithAllKnownScheme()).Build()
		reconciler := &Reconciler{cli: cli, recorder: record.NewFakeRecorder(10), instanceStatusClient: unreachableInstance{}}
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "payments-1", Namespace: namespace}}
		Expect(reconciler.createSnapshot(ctx, cluster, backup, pod, pvc.DeepCopy())).To(Succeed())

		var result volumesnapshotv1.VolumeSnapshot
		Expect(cli.Get(ctx, types.NamespacedName{Name: backup.Name, Namespace: namespace}, &result)).To(Succeed())
		return result
	}

	// The labels and annotations that do not come from the operator: those
	// of the PVC, the settings, the Cluster and the rules
	ownLabels := func(vs volumesnapshotv1.VolumeSnapshot) map[string]string {
		result := map[string]string{}
		for key, value := range vs.Labels {
			if !utils.IsOperatorMetadataKey(key) {
				result[key] = value
			}
		}
		return result
	}

	Context("without rules, from the Backup or the operator", func() {
		It("gives a VolumeSnapshot owned by the Cluster the metadata of the Cluster over the settings and the PVC",
			func(ctx SpecContext) {
				vs := snapshot(ctx, apiv1.SnapshotOwnerReferenceCluster)
				Expect(ownLabels(vs)).To(Equal(map[string]string{
					"pvc": "data", "pvc-and-config": "config", "config": "snapshot",
					"inherited": "cluster", "shared": "inherited",
				}))
				Expect(vs.Labels).To(HaveKeyWithValue(utils.ClusterLabelName, "payments"))
				Expect(vs.Annotations).To(HaveKeyWithValue("pvc-note", "data"))
				Expect(vs.Annotations).To(HaveKeyWithValue("config-note", "snapshot"))
				Expect(vs.Annotations).To(HaveKeyWithValue(utils.PvcRoleLabelName, string(utils.PVCRolePgData)))
				Expect(vs.OwnerReferences).To(HaveLen(1))
				Expect(vs.OwnerReferences[0].Kind).To(Equal(apiv1.ClusterKind))
			})

		DescribeTable("gives the other VolumeSnapshots the settings over the PVC, without the metadata of the Cluster",
			func(ctx SpecContext, owner apiv1.SnapshotOwnerReference, ownerKind string) {
				vs := snapshot(ctx, owner)
				Expect(ownLabels(vs)).To(Equal(map[string]string{
					"pvc": "data", "pvc-and-config": "config", "config": "snapshot", "shared": "config",
				}))
				if ownerKind == "" {
					Expect(vs.OwnerReferences).To(BeEmpty())
				} else {
					Expect(vs.OwnerReferences).To(HaveLen(1))
					Expect(vs.OwnerReferences[0].Kind).To(Equal(ownerKind))
				}
			},
			Entry("owned by the Backup", apiv1.SnapshotOwnerReferenceBackup, apiv1.BackupKind),
			Entry("not owned", apiv1.SnapshotOwnerReferenceNone, ""),
		)
	})

	Context("with rules", func() {
		DescribeTable("puts the rules of the Backup over every other source, below the mandatory metadata",
			func(ctx SpecContext, owner apiv1.SnapshotOwnerReference) {
				settings("", `[{"targets": ["volumeSnapshots"], "labels": {"mandatory": "operator"}}]`)
				backup.Spec.MetadataPropagation = []apiv1.MetadataPropagationRule{{
					Targets: []apiv1.MetadataPropagationTarget{"volumeSnapshots"},
					Labels: map[string]string{
						"pvc": "backup", "config": "backup", "shared": "backup", "mandatory": "backup",
					},
					Annotations: map[string]string{"pvc-note": "backup"},
				}}
				vs := snapshot(ctx, owner)
				Expect(vs.Labels).To(HaveKeyWithValue("pvc", "backup"))
				Expect(vs.Labels).To(HaveKeyWithValue("config", "backup"))
				Expect(vs.Labels).To(HaveKeyWithValue("shared", "backup"))
				Expect(vs.Labels).To(HaveKeyWithValue("mandatory", "operator"))
				Expect(vs.Annotations).To(HaveKeyWithValue("pvc-note", "backup"))
				// The other annotations stay
				Expect(vs.Annotations).To(HaveKeyWithValue("config-note", "snapshot"))
				Expect(vs.Annotations).To(HaveKey(utils.ClusterManifestAnnotationName))
			},
			Entry("owned by the Cluster", apiv1.SnapshotOwnerReferenceCluster),
			Entry("owned by the Backup", apiv1.SnapshotOwnerReferenceBackup),
			Entry("not owned", apiv1.SnapshotOwnerReferenceNone),
		)

		DescribeTable("puts the default metadata below the PVC, the settings and the Cluster",
			func(ctx SpecContext, target string) {
				settings(`[{"targets": ["`+target+`"], "labels": {"pvc": "default", "config": "default",
					"inherited": "default", "default": "operator"}}]`, "")
				vs := snapshot(ctx, apiv1.SnapshotOwnerReferenceCluster)
				Expect(vs.Labels).To(HaveKeyWithValue("pvc", "data"))
				Expect(vs.Labels).To(HaveKeyWithValue("config", "snapshot"))
				Expect(vs.Labels).To(HaveKeyWithValue("inherited", "cluster"))
				Expect(vs.Labels).To(HaveKeyWithValue("default", "operator"))
			},
			Entry("for the VolumeSnapshots", "volumeSnapshots"),
			Entry("for every object", "all"),
		)

		It("gives a VolumeSnapshot owned by the Cluster the rules of the Cluster for all, below those of the Backup",
			func(ctx SpecContext) {
				cluster.Spec.MetadataPropagation = []apiv1.MetadataPropagationRule{{
					Targets: []apiv1.MetadataPropagationTarget{"all"},
					Labels:  map[string]string{"cluster-rule": "all", "both": "cluster"},
				}, {
					Targets: []apiv1.MetadataPropagationTarget{"persistentVolumeClaims"},
					Labels:  map[string]string{"pvc-rule": "x"},
				}}
				backup.Spec.MetadataPropagation = []apiv1.MetadataPropagationRule{{
					Targets: []apiv1.MetadataPropagationTarget{"volumeSnapshots"},
					Labels:  map[string]string{"both": "backup"},
				}}
				vs := snapshot(ctx, apiv1.SnapshotOwnerReferenceCluster)
				Expect(vs.Labels).To(HaveKeyWithValue("cluster-rule", "all"))
				Expect(vs.Labels).To(HaveKeyWithValue("both", "backup"))
				Expect(vs.Labels).NotTo(HaveKey("pvc-rule"))
			})

		DescribeTable("takes the operator rules naming the Backup, not those naming the Cluster, whatever the owner",
			func(ctx SpecContext, owner apiv1.SnapshotOwnerReference) {
				settings(`[{"resources": ["Cluster"], "targets": ["all"], "labels": {"cluster-default": "x"}}]`,
					`[{"resources": ["Cluster", "ScheduledBackup"], "targets": ["all"], "labels": {"shared": "cluster"}},
					  {"resources": ["Backup"], "targets": ["all"], "labels": {"backup-mandatory": "y"}}]`)
				vs := snapshot(ctx, owner)
				Expect(vs.Labels).To(HaveKeyWithValue("backup-mandatory", "y"))
				Expect(vs.Labels).NotTo(HaveKey("cluster-default"))
				Expect(vs.Labels).NotTo(HaveKeyWithValue("shared", "cluster"))
			},
			Entry("owned by the Cluster", apiv1.SnapshotOwnerReferenceCluster),
			Entry("owned by the Backup", apiv1.SnapshotOwnerReferenceBackup),
			Entry("not owned", apiv1.SnapshotOwnerReferenceNone),
		)

		It("never replaces the keys the operator sets itself", func(ctx SpecContext) {
			backup.Spec.MetadataPropagation = []apiv1.MetadataPropagationRule{{
				Targets: []apiv1.MetadataPropagationTarget{"volumeSnapshots"},
				Labels: map[string]string{
					utils.BackupNameLabelName: "other", utils.ClusterLabelName: "other",
				},
				Annotations: map[string]string{utils.PvcRoleLabelName: "other"},
			}}
			vs := snapshot(ctx, apiv1.SnapshotOwnerReferenceCluster)
			Expect(vs.Labels).To(HaveKeyWithValue(utils.BackupNameLabelName, "nightly"))
			Expect(vs.Labels).To(HaveKeyWithValue(utils.ClusterLabelName, "payments"))
			Expect(vs.Annotations).To(HaveKeyWithValue(utils.PvcRoleLabelName, string(utils.PVCRolePgData)))
		})
	})
})
