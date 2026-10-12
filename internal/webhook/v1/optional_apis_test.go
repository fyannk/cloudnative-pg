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
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/internal/configuration"
	"github.com/cloudnative-pg/cloudnative-pg/internal/webhook/guard"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// An object asking for an optional API the operator does not use is refused
// when it cannot work without it, on creation or when the field changes, and
// only warned about when the field is unchanged or when it works without the
// API. With every API available, nothing changes.
var _ = Describe("Objects asking for an unavailable optional API", func() {
	BeforeEach(func() {
		previous := configuration.Current
		configuration.Current = configuration.NewConfiguration()
		hadVolumeSnapshot := utils.HaveVolumeSnapshot()
		DeferCleanup(func() {
			configuration.Current = previous
			utils.SetVolumeSnapshot(hadVolumeSnapshot)
		})
		utils.SetVolumeSnapshot(true)
	})

	// A real admission request carries the request in its context; the
	// admission guard of the reconcilers calls ValidateCreate without it
	admissionContext := func(ctx context.Context) context.Context {
		return admission.NewContextWithRequest(ctx, admission.Request{})
	}

	disableAll := func() {
		configuration.Current.APIClusterImageCatalogEnabled = false
		configuration.Current.APIPodMonitorEnabled = false
		utils.SetVolumeSnapshot(false)
	}
	catalogRef := func(kind, name string) *apiv1.ImageCatalogRef {
		return &apiv1.ImageCatalogRef{
			TypedLocalObjectReference: corev1.TypedLocalObjectReference{
				APIGroup: ptr.To(apiv1.SchemeGroupVersion.Group), Kind: kind, Name: name,
			},
			Major: 17,
		}
	}
	clusterAskingForEverything := func() *apiv1.Cluster {
		return &apiv1.Cluster{Spec: apiv1.ClusterSpec{
			ImageCatalogRef: catalogRef(apiv1.ClusterImageCatalogKind, "postgresql"),
			Bootstrap: &apiv1.BootstrapConfiguration{Recovery: &apiv1.BootstrapRecovery{
				VolumeSnapshots: &apiv1.DataSource{
					Storage: corev1.TypedLocalObjectReference{Name: "snap", Kind: "VolumeSnapshot"},
				},
			}},
			Backup:     &apiv1.BackupConfiguration{VolumeSnapshot: &apiv1.VolumeSnapshotConfiguration{}},
			Monitoring: &apiv1.MonitoringConfiguration{EnablePodMonitor: true}, //nolint:staticcheck
		}}
	}

	Context("a Cluster", func() {
		It("is not affected while every API is available, on creation or update", func() {
			cluster := clusterAskingForEverything()
			errs, warnings := getClusterUnavailableAPIProblems(cluster, nil)
			Expect(errs).To(BeEmpty())
			Expect(warnings).To(BeEmpty())
			errs, warnings = getClusterUnavailableAPIProblems(cluster, clusterAskingForEverything())
			Expect(errs).To(BeEmpty())
			Expect(warnings).To(BeEmpty())
		})

		It("asking for nothing optional is not affected when every API is unavailable", func() {
			disableAll()
			cluster := &apiv1.Cluster{Spec: apiv1.ClusterSpec{
				ImageCatalogRef: catalogRef(apiv1.ImageCatalogKind, "postgresql"),
				Monitoring:      &apiv1.MonitoringConfiguration{EnablePodMonitor: false}, //nolint:staticcheck
			}}
			errs, warnings := getClusterUnavailableAPIProblems(cluster, nil)
			Expect(errs).To(BeEmpty())
			Expect(warnings).To(BeEmpty())
		})

		It("is refused on creation for what it cannot work without, and warned about the rest", func() {
			disableAll()
			errs, warnings := getClusterUnavailableAPIProblems(clusterAskingForEverything(), nil)
			Expect(errs).To(HaveLen(2))
			Expect(errs[0].Field).To(Equal("spec.imageCatalogRef"))
			Expect(errs[0].Detail).To(ContainSubstring("the ClusterImageCatalog API is not available to this operator"))
			Expect(errs[0].Detail).To(ContainSubstring("API_CLUSTER_IMAGE_CATALOG_ENABLED"))
			Expect(errs[0].Detail).To(ContainSubstring("use a namespaced ImageCatalog"))
			Expect(errs[1].Field).To(Equal("spec.bootstrap.recovery.volumeSnapshots"))
			Expect(errs[1].Detail).To(ContainSubstring("API_VOLUME_SNAPSHOT_ENABLED"))
			Expect(warnings).To(HaveLen(2))
			Expect(warnings[0]).To(HavePrefix("spec.backup.volumeSnapshot: the VolumeSnapshot API is not available"))
			Expect(warnings[1]).To(HavePrefix("spec.monitoring.enablePodMonitor: the PodMonitor API is not available"))
		})

		It("is only warned about when updated without changing those fields", func() {
			disableAll()
			errs, warnings := getClusterUnavailableAPIProblems(clusterAskingForEverything(), clusterAskingForEverything())
			Expect(errs).To(BeEmpty())
			Expect(warnings).To(HaveLen(4))
			Expect(warnings[0]).To(HavePrefix("spec.imageCatalogRef: the ClusterImageCatalog API"))
			Expect(warnings[1]).To(HavePrefix("spec.bootstrap.recovery.volumeSnapshots: the VolumeSnapshot API"))
		})

		DescribeTable("is refused when an update newly points to a ClusterImageCatalog",
			func(old *apiv1.ImageCatalogRef) {
				configuration.Current.APIClusterImageCatalogEnabled = false
				cluster := &apiv1.Cluster{Spec: apiv1.ClusterSpec{
					ImageCatalogRef: catalogRef(apiv1.ClusterImageCatalogKind, "postgresql"),
				}}
				errs, _ := getClusterUnavailableAPIProblems(cluster, &apiv1.Cluster{Spec: apiv1.ClusterSpec{
					ImageCatalogRef: old,
				}})
				Expect(errs).To(HaveLen(1))
				Expect(errs[0].Field).To(Equal("spec.imageCatalogRef"))
			},
			Entry("from an image name", nil),
			Entry("from a namespaced ImageCatalog", catalogRef(apiv1.ImageCatalogKind, "postgresql")),
			Entry("from another ClusterImageCatalog", catalogRef(apiv1.ClusterImageCatalogKind, "other")),
		)

		It("is refused through ValidateCreate, with the warnings", func(ctx SpecContext) {
			disableAll()
			v := &ClusterCustomValidator{}
			cluster := clusterAskingForEverything()
			cluster.Name = "payments"
			warnings, err := v.ValidateCreate(admissionContext(ctx), cluster)
			Expect(err).To(MatchError(ContainSubstring("spec.imageCatalogRef")))
			Expect(err).To(MatchError(ContainSubstring("spec.bootstrap.recovery.volumeSnapshots")))
			Expect(warnings).To(ContainElement(HavePrefix("spec.monitoring.enablePodMonitor:")))
		})

		It("is warned about, and the unchanged fields are not refused, through ValidateUpdate", func() {
			disableAll()
			v := &ClusterCustomValidator{}
			warnings, err := v.ValidateUpdate(context.Background(), clusterAskingForEverything(),
				clusterAskingForEverything())
			if err != nil {
				Expect(err).NotTo(MatchError(ContainSubstring("not available to this operator")))
			}
			Expect(warnings).To(ContainElement(HavePrefix("spec.imageCatalogRef: the ClusterImageCatalog API")))
		})
	})

	// The admission guard validates the stored objects at every
	// reconciliation through ValidateCreate: an object admitted before the
	// API was disabled must not be marked invalid, which would stop its
	// reconciliation, as for a Cluster bootstrapped from snapshots long ago
	Context("a stored object, validated again by the admission guard", func() {
		It("is only warned about through ValidateCreate without an admission request", func(ctx SpecContext) {
			disableAll()
			warnings, err := (&ClusterCustomValidator{}).ValidateCreate(ctx, clusterAskingForEverything())
			if err != nil {
				Expect(err).NotTo(MatchError(ContainSubstring("not available to this operator")))
			}
			Expect(warnings).To(ContainElement(HavePrefix("spec.imageCatalogRef: the ClusterImageCatalog API")))
			Expect(warnings).To(ContainElement(HavePrefix("spec.bootstrap.recovery.volumeSnapshots: the VolumeSnapshot API")))

			poolerWarnings, err := (&PoolerCustomValidator{}).ValidateCreate(ctx, &apiv1.Pooler{Spec: apiv1.PoolerSpec{
				Cluster: apiv1.LocalObjectReference{Name: "payments"},
				PgBouncer: &apiv1.PgBouncerSpec{
					PoolMode: apiv1.PgBouncerPoolModeSession,
					ImageCatalogRef: &apiv1.ImageCatalogComponentRef{
						TypedLocalObjectReference: corev1.TypedLocalObjectReference{
							APIGroup: ptr.To(apiv1.SchemeGroupVersion.Group), Kind: apiv1.ClusterImageCatalogKind, Name: "c",
						},
						Key: "pgbouncer",
					},
				},
			}})
			Expect(err).ToNot(HaveOccurred())
			Expect(poolerWarnings).To(ContainElement(HavePrefix("spec.pgbouncer.imageCatalogRef:")))

			backupWarnings, err := (&BackupCustomValidator{}).ValidateCreate(ctx, &apiv1.Backup{
				Spec: apiv1.BackupSpec{Method: apiv1.BackupMethodVolumeSnapshot},
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(backupWarnings).To(ConsistOf(HavePrefix("spec.method: the VolumeSnapshot API")))
		})

		It("keeps a Cluster bootstrapped from snapshots reconciling through the real guard", func(ctx SpecContext) {
			disableAll()
			cluster := &apiv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "restored", Namespace: "default"},
				Spec: apiv1.ClusterSpec{
					Instances:            1,
					ImageName:            "ghcr.io/cloudnative-pg/postgresql:18",
					StorageConfiguration: apiv1.StorageConfiguration{Size: "1Gi"},
					Bootstrap: &apiv1.BootstrapConfiguration{Recovery: &apiv1.BootstrapRecovery{
						VolumeSnapshots: &apiv1.DataSource{Storage: corev1.TypedLocalObjectReference{
							APIGroup: ptr.To("snapshot.storage.k8s.io"), Kind: "VolumeSnapshot", Name: "snap",
						}},
					}},
				},
			}
			// A stored Cluster is already defaulted: the guard only validates it
			Expect((&ClusterCustomDefaulter{}).Default(ctx, cluster)).To(Succeed())
			_, validationErr := (&ClusterCustomValidator{}).ValidateCreate(ctx, cluster)
			Expect(validationErr).ToNot(HaveOccurred())
			result, err := NewClusterAdmissionGuard().EnsureResourceIsAdmitted(ctx,
				guard.AdmissionParams[*apiv1.Cluster]{Object: cluster, ApplyChanges: false})
			Expect(err).ToNot(HaveOccurred())
			Expect(result.IsZero()).To(BeTrue(), "the guard requeues an invalid object")
			Expect(cluster.Status.Phase).NotTo(Equal(apiv1.PhaseDefinitionInvalid))

			// The same Cluster, created now, is refused
			_, err = (&ClusterCustomValidator{}).ValidateCreate(admissionContext(ctx), cluster)
			Expect(err).To(MatchError(ContainSubstring("spec.bootstrap.recovery.volumeSnapshots")))
		})
	})

	Context("a Pooler", func() {
		pooler := func(kind string, podMonitor bool) *apiv1.Pooler {
			result := &apiv1.Pooler{Spec: apiv1.PoolerSpec{
				Cluster:    apiv1.LocalObjectReference{Name: "payments"},
				PgBouncer:  &apiv1.PgBouncerSpec{PoolMode: apiv1.PgBouncerPoolModeSession},
				Monitoring: &apiv1.PoolerMonitoringConfiguration{EnablePodMonitor: podMonitor}, //nolint:staticcheck
			}}
			if kind != "" {
				result.Spec.PgBouncer.ImageCatalogRef = &apiv1.ImageCatalogComponentRef{
					TypedLocalObjectReference: corev1.TypedLocalObjectReference{
						APIGroup: ptr.To(apiv1.SchemeGroupVersion.Group), Kind: kind, Name: "c",
					},
					Key: "pgbouncer",
				}
			}
			return result
		}

		It("is not affected while every API is available", func() {
			errs, warnings := getPoolerUnavailableAPIProblems(pooler(apiv1.ClusterImageCatalogKind, true), nil)
			Expect(errs).To(BeEmpty())
			Expect(warnings).To(BeEmpty())
		})

		It("is refused on creation for a ClusterImageCatalog, and warned about its PodMonitor", func() {
			disableAll()
			errs, warnings := getPoolerUnavailableAPIProblems(pooler(apiv1.ClusterImageCatalogKind, true), nil)
			Expect(errs).To(HaveLen(1))
			Expect(errs[0].Field).To(Equal("spec.pgbouncer.imageCatalogRef"))
			Expect(warnings).To(ConsistOf(HavePrefix("spec.monitoring.enablePodMonitor: the PodMonitor API")))
		})

		It("is only warned about when updated without changing the reference", func() {
			disableAll()
			errs, warnings := getPoolerUnavailableAPIProblems(pooler(apiv1.ClusterImageCatalogKind, false),
				pooler(apiv1.ClusterImageCatalogKind, false))
			Expect(errs).To(BeEmpty())
			Expect(warnings).To(ConsistOf(HavePrefix("spec.pgbouncer.imageCatalogRef: the ClusterImageCatalog API")))
		})

		It("is refused when an update changes the reference", func() {
			disableAll()
			errs, _ := getPoolerUnavailableAPIProblems(pooler(apiv1.ClusterImageCatalogKind, false),
				pooler(apiv1.ImageCatalogKind, false))
			Expect(errs).To(HaveLen(1))
		})

		It("is not affected by a namespaced ImageCatalog", func() {
			disableAll()
			errs, warnings := getPoolerUnavailableAPIProblems(pooler(apiv1.ImageCatalogKind, false), nil)
			Expect(errs).To(BeEmpty())
			Expect(warnings).To(BeEmpty())
		})

		It("goes through ValidateCreate and ValidateUpdate", func(ctx SpecContext) {
			disableAll()
			v := &PoolerCustomValidator{}
			_, err := v.ValidateCreate(admissionContext(ctx), pooler(apiv1.ClusterImageCatalogKind, false))
			Expect(err).To(MatchError(ContainSubstring("spec.pgbouncer.imageCatalogRef")))
			warnings, err := v.ValidateUpdate(ctx, pooler(apiv1.ClusterImageCatalogKind, false),
				pooler(apiv1.ClusterImageCatalogKind, false))
			Expect(err).ToNot(HaveOccurred())
			Expect(warnings).To(ContainElement(HavePrefix("spec.pgbouncer.imageCatalogRef:")))
		})
	})

	Context("a Backup or a ScheduledBackup with the volumeSnapshot method", func() {
		snapshotBackup := func() *apiv1.Backup {
			return &apiv1.Backup{Spec: apiv1.BackupSpec{Method: apiv1.BackupMethodVolumeSnapshot}}
		}
		snapshotSchedule := func(method apiv1.BackupMethod) *apiv1.ScheduledBackup {
			return &apiv1.ScheduledBackup{Spec: apiv1.ScheduledBackupSpec{Schedule: "0 0 0 * * *", Method: method}}
		}

		It("is refused on creation, naming the setting and the restart", func(ctx SpecContext) {
			utils.SetVolumeSnapshot(false)
			_, err := (&BackupCustomValidator{}).ValidateCreate(admissionContext(ctx), snapshotBackup())
			Expect(err).To(MatchError(ContainSubstring("the VolumeSnapshot API is not available to this operator")))
			Expect(err).To(MatchError(ContainSubstring("API_VOLUME_SNAPSHOT_ENABLED")))
			Expect(err).To(MatchError(ContainSubstring("restart the operator")))
			_, err = (&ScheduledBackupCustomValidator{}).ValidateCreate(admissionContext(ctx),
				snapshotSchedule(apiv1.BackupMethodVolumeSnapshot))
			Expect(err).To(MatchError(ContainSubstring("API_VOLUME_SNAPSHOT_ENABLED")))
		})

		It("is only warned about when updated without changing the method", func(ctx SpecContext) {
			utils.SetVolumeSnapshot(false)
			warnings, err := (&BackupCustomValidator{}).ValidateUpdate(ctx, snapshotBackup(), snapshotBackup())
			Expect(err).ToNot(HaveOccurred())
			Expect(warnings).To(ConsistOf(HavePrefix("spec.method: the VolumeSnapshot API is not available")))

			warnings, err = (&ScheduledBackupCustomValidator{}).ValidateUpdate(ctx,
				snapshotSchedule(apiv1.BackupMethodVolumeSnapshot), snapshotSchedule(apiv1.BackupMethodVolumeSnapshot))
			Expect(err).ToNot(HaveOccurred())
			Expect(warnings).To(ContainElement(HavePrefix("spec.method: the VolumeSnapshot API is not available")))
		})

		It("is refused when an update switches to the volumeSnapshot method", func(ctx SpecContext) {
			utils.SetVolumeSnapshot(false)
			_, err := (&ScheduledBackupCustomValidator{}).ValidateUpdate(ctx,
				snapshotSchedule(apiv1.BackupMethodBarmanObjectStore), snapshotSchedule(apiv1.BackupMethodVolumeSnapshot))
			Expect(err).To(MatchError(ContainSubstring("API_VOLUME_SNAPSHOT_ENABLED")))
		})

		It("keeps the other errors of an unchanged update", func(ctx SpecContext) {
			utils.SetVolumeSnapshot(false)
			schedule := snapshotSchedule(apiv1.BackupMethodVolumeSnapshot)
			schedule.Spec.Schedule = "not a schedule"
			_, err := (&ScheduledBackupCustomValidator{}).ValidateUpdate(ctx,
				snapshotSchedule(apiv1.BackupMethodVolumeSnapshot), schedule)
			Expect(err).To(MatchError(ContainSubstring("spec.schedule")))
			Expect(err).NotTo(MatchError(ContainSubstring("not available to this operator")))
		})

		It("is accepted while the API is available", func(ctx SpecContext) {
			warnings, err := (&BackupCustomValidator{}).ValidateUpdate(ctx, snapshotBackup(), snapshotBackup())
			Expect(err).ToNot(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})
	})
})
