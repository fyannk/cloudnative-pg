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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/internal/scheme"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/postgres"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/specs"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	autoGrowCluster = "grow"
	gibibyte        = uint64(1024 * 1024 * 1024)
)

// autoGrowPVC is the PVC of an instance for a volume, with its request and
// the capacity it got
func autoGrowPVC(
	serial string,
	calculator ExpectedObjectCalculator,
	request, capacity string,
) corev1.PersistentVolumeClaim {
	instance := autoGrowCluster + "-" + serial
	labels := calculator.GetLabels(instance)
	labels[utils.ClusterLabelName] = autoGrowCluster
	return corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      calculator.GetName(instance),
			Namespace: "default",
			Labels:    labels,
			Annotations: map[string]string{
				utils.ClusterSerialAnnotationName: serial,
				utils.PVCStatusAnnotationName:     StatusReady,
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse(request)},
			},
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Capacity: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse(capacity)},
		},
	}
}

// reporting is the status of an instance reporting the usage of its volumes
func reporting(serial string, usages ...postgres.VolumeUsage) postgres.PostgresqlStatus {
	return postgres.PostgresqlStatus{
		Pod:          &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: autoGrowCluster + "-" + serial}},
		VolumeUsages: usages,
	}
}

// outOfWALSpace is the status of an instance whose PostgreSQL stopped
// because the WAL volume is full: it cannot report its usage
func outOfWALSpace(serial string) postgres.PostgresqlStatus {
	status := reporting(serial)
	status.Pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name: specs.PostgresContainerName,
		State: corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{ExitCode: apiv1.MissingWALDiskSpaceExitCode},
		},
	}}
	status.Error = context.DeadlineExceeded
	return status
}

// used is the usage of a filesystem of the given size in GiB, used at the
// given percentage
func used(volume string, sizeGiB uint64, percent uint64) postgres.VolumeUsage {
	total := sizeGiB * gibibyte
	return postgres.VolumeUsage{
		Volume:         volume,
		TotalBytes:     total,
		AvailableBytes: total - total*percent/100,
		UsedBytes:      total * percent / 100,
	}
}

func instancesReporting(items ...postgres.PostgresqlStatus) postgres.PostgresqlStatusList {
	return postgres.PostgresqlStatusList{Items: items}
}

// sizesOf returns the sizes planned for each volume, as strings: quantities
// computed and parsed differ in their cached representation
func sizesOf(plan autoGrowPlan) map[pvcGroup]string {
	result := make(map[pvcGroup]string, len(plan.sizes))
	for group, size := range plan.sizes {
		result[group] = size.String()
	}
	return result
}

func quantity(value string) resource.Quantity {
	return resource.MustParse(value)
}

var _ = Describe("PVC groups", func() {
	It("returns the largest request of each volume, terminating PVCs excluded", func() {
		terminating := autoGrowPVC("3", NewPgDataCalculator(), "9Gi", "9Gi")
		terminating.DeletionTimestamp = ptr.To(metav1.Now())
		pvcs := []corev1.PersistentVolumeClaim{
			autoGrowPVC("1", NewPgDataCalculator(), "2Gi", "2Gi"),
			autoGrowPVC("2", NewPgDataCalculator(), "3Gi", "3Gi"),
			terminating,
			autoGrowPVC("1", NewPgWalCalculator(), "1Gi", "1Gi"),
			autoGrowPVC("1", NewPgTablespaceCalculator("alpha"), "4Gi", "4Gi"),
			autoGrowPVC("2", NewPgTablespaceCalculator("beta"), "5Gi", "5Gi"),
		}

		largest := getLargestRequests(pvcs)

		Expect(largest).To(HaveLen(4))
		Expect(largest[pvcGroup{role: utils.PVCRolePgData}]).To(Equal(quantity("3Gi")))
		Expect(largest[pvcGroup{role: utils.PVCRolePgWal}]).To(Equal(quantity("1Gi")))
		Expect(largest[pvcGroup{role: utils.PVCRolePgTablespace, tablespace: "alpha"}]).To(Equal(quantity("4Gi")))
		Expect(largest[pvcGroup{role: utils.PVCRolePgTablespace, tablespace: "beta"}]).To(Equal(quantity("5Gi")))
	})

	It("raises the size of a new PVC to the largest request of its volume, never lowers it", func() {
		group := pvcGroup{role: utils.PVCRolePgData}
		largest := map[pvcGroup]resource.Quantity{group: quantity("3Gi")}

		Expect(withGroupSize(apiv1.StorageConfiguration{Size: "1Gi"}, group, largest).Size).To(Equal("3Gi"))
		Expect(withGroupSize(apiv1.StorageConfiguration{Size: "5Gi"}, group, largest).Size).To(Equal("5Gi"))
		Expect(withGroupSize(apiv1.StorageConfiguration{Size: "1Gi"}, group, nil).Size).To(Equal("1Gi"))

		// a size given only by the template is raised too
		fromTemplate := apiv1.StorageConfiguration{PersistentVolumeClaimTemplate: &corev1.PersistentVolumeClaimSpec{
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: quantity("1Gi")},
			},
		}}
		raised := withGroupSize(fromTemplate, group, largest)
		Expect(raised.GetSizeOrNil()).To(Equal(ptr.To(quantity("3Gi"))))
	})

	It("creates the missing PVC of an instance at the size of its volume", func() {
		cluster := &apiv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: autoGrowCluster, Namespace: "default"},
			Spec: apiv1.ClusterSpec{
				Instances:            2,
				StorageConfiguration: apiv1.StorageConfiguration{Size: "1Gi"},
				WalStorage:           &apiv1.StorageConfiguration{Size: "1Gi"},
			},
		}
		grown := autoGrowPVC("1", NewPgDataCalculator(), "3Gi", "3Gi")
		wal := autoGrowPVC("1", NewPgWalCalculator(), "1Gi", "1Gi")
		cli := fake.NewClientBuilder().WithScheme(scheme.BuildWithAllKnownScheme()).
			WithObjects(cluster, &grown, &wal).Build()

		Expect(CreateInstancePVCs(context.Background(), cli, cluster, nil, 2)).To(Succeed())

		var data, walOfNewInstance corev1.PersistentVolumeClaim
		Expect(cli.Get(context.Background(),
			client.ObjectKey{Namespace: "default", Name: autoGrowCluster + "-2"}, &data)).To(Succeed())
		Expect(data.Spec.Resources.Requests.Storage()).To(Equal(ptr.To(quantity("3Gi"))))
		Expect(cli.Get(context.Background(),
			client.ObjectKey{Namespace: "default", Name: autoGrowCluster + "-2-wal"}, &walOfNewInstance)).To(Succeed())
		Expect(walOfNewInstance.Spec.Resources.Requests.Storage()).To(Equal(ptr.To(quantity("1Gi"))))
	})
})

var _ = Describe("autoGrow plan", func() {
	var cluster *apiv1.Cluster
	pgData := pvcGroup{role: utils.PVCRolePgData}
	wal := pvcGroup{role: utils.PVCRolePgWal}
	tablespace := pvcGroup{role: utils.PVCRolePgTablespace, tablespace: "alpha"}

	BeforeEach(func() {
		cluster = &apiv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: autoGrowCluster, Namespace: "default"},
			Spec: apiv1.ClusterSpec{
				Instances: 2,
				StorageConfiguration: apiv1.StorageConfiguration{
					Size:     "10Gi",
					AutoGrow: &apiv1.AutoGrowConfiguration{MaxSize: "20Gi"},
				},
			},
		}
	})

	dataPVCs := func(request string) []corev1.PersistentVolumeClaim {
		return []corev1.PersistentVolumeClaim{
			autoGrowPVC("1", NewPgDataCalculator(), request, request),
			autoGrowPVC("2", NewPgDataCalculator(), request, request),
		}
	}

	It("plans nothing for a cluster without autoGrow", func() {
		Expect(HasAutoGrow(cluster)).To(BeTrue())
		cluster.Spec.StorageConfiguration.AutoGrow = nil
		Expect(HasAutoGrow(cluster)).To(BeFalse())

		plan, err := planAutoGrow(cluster, dataPVCs("10Gi"), instancesReporting(reporting("1", used("pgdata", 10, 99))))

		Expect(err).ToNot(HaveOccurred())
		Expect(plan.sizes).To(BeEmpty())
		Expect(plan.steps).To(BeEmpty())
		Expect(plan.stops).To(BeEmpty())
	})

	It("keeps the size below the threshold", func() {
		plan, err := planAutoGrow(cluster, dataPVCs("10Gi"), instancesReporting(
			reporting("1", used("pgdata", 10, 79)),
			reporting("2", used("pgdata", 10, 50)),
		))

		Expect(err).ToNot(HaveOccurred())
		Expect(sizesOf(plan)).To(Equal(map[pvcGroup]string{pgData: "10Gi"}))
		Expect(plan.steps).To(BeEmpty())
		Expect(plan.stops).To(BeEmpty())
	})

	It("grows every PVC of the volume by one step when the most used instance crosses the threshold", func() {
		plan, err := planAutoGrow(cluster, dataPVCs("10Gi"), instancesReporting(
			reporting("1", used("pgdata", 10, 50)),
			reporting("2", used("pgdata", 10, 85)),
		))

		Expect(err).ToNot(HaveOccurred())
		Expect(sizesOf(plan)[pgData]).To(Equal("12Gi"))
		Expect(plan.steps).To(HaveLen(1))
		Expect(plan.steps[0].from.String()).To(Equal("10Gi"))
		Expect(plan.steps[0].instance).To(Equal(autoGrowCluster + "-2"))
		Expect(plan.steps[0].usedPercent).To(BeNumerically("~", 85, 0.1))
	})

	It("grows by a quantity, and never beyond maxSize", func() {
		cluster.Spec.StorageConfiguration.AutoGrow.Step = "8Gi"

		plan, err := planAutoGrow(cluster, dataPVCs("15Gi"), instancesReporting(reporting("1", used("pgdata", 15, 90))))

		Expect(err).ToNot(HaveOccurred())
		Expect(sizesOf(plan)[pgData]).To(Equal("20Gi"))
	})

	It("stops at maxSize and says so", func() {
		plan, err := planAutoGrow(cluster, dataPVCs("20Gi"), instancesReporting(reporting("1", used("pgdata", 20, 90))))

		Expect(err).ToNot(HaveOccurred())
		Expect(sizesOf(plan)[pgData]).To(Equal("20Gi"))
		Expect(plan.steps).To(BeEmpty())
		Expect(plan.stops).To(ConsistOf(autoGrowStop{
			group: pgData, reason: apiv1.ConditionReasonMaxSizeReached, size: quantity("20Gi"), threshold: 80,
		}))
	})

	It("starts from the largest PVC of the volume and brings the others to it", func() {
		pvcs := []corev1.PersistentVolumeClaim{
			autoGrowPVC("1", NewPgDataCalculator(), "14Gi", "14Gi"),
			autoGrowPVC("2", NewPgDataCalculator(), "10Gi", "10Gi"),
		}

		plan, err := planAutoGrow(cluster, pvcs, instancesReporting(reporting("1", used("pgdata", 14, 10))))

		Expect(err).ToNot(HaveOccurred())
		Expect(sizesOf(plan)[pgData]).To(Equal("14Gi"))
		Expect(plan.steps).To(BeEmpty())
	})

	It("waits until every PVC of the volume got the size of the previous step", func() {
		pvcs := dataPVCs("12Gi")
		pvcs[1].Status.Capacity[corev1.ResourceStorage] = quantity("10Gi")

		plan, err := planAutoGrow(cluster, pvcs, instancesReporting(reporting("1", used("pgdata", 12, 95))))
		Expect(err).ToNot(HaveOccurred())
		Expect(sizesOf(plan)[pgData]).To(Equal("12Gi"))
		Expect(plan.steps).To(BeEmpty())

		pvcs = dataPVCs("12Gi")
		pvcs[0].Status.Conditions = []corev1.PersistentVolumeClaimCondition{{
			Type: corev1.PersistentVolumeClaimFileSystemResizePending, Status: corev1.ConditionTrue,
		}}
		plan, err = planAutoGrow(cluster, pvcs, instancesReporting(reporting("1", used("pgdata", 12, 95))))
		Expect(err).ToNot(HaveOccurred())
		Expect(plan.steps).To(BeEmpty())
	})

	It("ignores a filesystem that is not the one of the PVC", func() {
		// local-path: the volume is a directory of the node filesystem
		plan, err := planAutoGrow(cluster, dataPVCs("10Gi"), instancesReporting(reporting("1", used("pgdata", 500, 95))))

		Expect(err).ToNot(HaveOccurred())
		Expect(plan.steps).To(BeEmpty())
		Expect(plan.stops).To(BeEmpty())
	})

	It("grows the volume holding the WAL files of an instance stopped for lack of WAL space", func() {
		plan, err := planAutoGrow(cluster, dataPVCs("10Gi"), instancesReporting(outOfWALSpace("1")))

		Expect(err).ToNot(HaveOccurred())
		Expect(sizesOf(plan)[pgData]).To(Equal("12Gi"))
		Expect(plan.steps).To(HaveLen(1))
		Expect(plan.steps[0].usedPercent).To(BeEquivalentTo(100))
	})

	It("does not grow the volume holding the WAL files while the WAL archiving fails", func() {
		cluster.Status.Conditions = []metav1.Condition{{
			Type:   string(apiv1.ConditionContinuousArchiving),
			Status: metav1.ConditionFalse,
			Reason: string(apiv1.ConditionReasonContinuousArchivingFailing),
		}}

		plan, err := planAutoGrow(cluster, dataPVCs("10Gi"), instancesReporting(reporting("1", used("pgdata", 10, 90))))

		Expect(err).ToNot(HaveOccurred())
		Expect(sizesOf(plan)[pgData]).To(Equal("10Gi"))
		Expect(plan.steps).To(BeEmpty())
		Expect(plan.stops).To(ConsistOf(autoGrowStop{
			group: pgData, reason: apiv1.ConditionReasonWALArchivingFailing, size: quantity("10Gi"), threshold: 80,
		}))
	})

	It("grows each volume from its own section and usage, PGDATA despite failing archiving", func() {
		cluster.Spec.WalStorage = &apiv1.StorageConfiguration{
			Size:     "2Gi",
			AutoGrow: &apiv1.AutoGrowConfiguration{MaxSize: "4Gi", Threshold: 60, Step: "1Gi"},
		}
		cluster.Spec.Tablespaces = []apiv1.TablespaceConfiguration{
			{Name: "alpha", Storage: apiv1.StorageConfiguration{
				Size:     "5Gi",
				AutoGrow: &apiv1.AutoGrowConfiguration{MaxSize: "50Gi", Step: "100%"},
			}},
			{Name: "beta", Storage: apiv1.StorageConfiguration{Size: "5Gi"}},
		}
		cluster.Status.Conditions = []metav1.Condition{{
			Type:   string(apiv1.ConditionContinuousArchiving),
			Status: metav1.ConditionFalse,
			Reason: string(apiv1.ConditionReasonContinuousArchivingFailing),
		}}
		pvcs := append(dataPVCs("10Gi"),
			autoGrowPVC("1", NewPgWalCalculator(), "2Gi", "2Gi"),
			autoGrowPVC("1", NewPgTablespaceCalculator("alpha"), "5Gi", "5Gi"),
			autoGrowPVC("1", NewPgTablespaceCalculator("beta"), "5Gi", "5Gi"),
		)

		plan, err := planAutoGrow(cluster, pvcs, instancesReporting(reporting("1",
			used("pgdata", 10, 85),
			used("wal", 2, 70),
			used("tbs-alpha", 5, 81),
			used("tbs-beta", 5, 99),
		)))

		Expect(HasAutoGrow(&apiv1.Cluster{Spec: apiv1.ClusterSpec{Tablespaces: cluster.Spec.Tablespaces}})).To(BeTrue())
		Expect(err).ToNot(HaveOccurred())
		Expect(sizesOf(plan)).To(Equal(map[pvcGroup]string{
			pgData:     "12Gi",
			wal:        "2Gi",
			tablespace: "10Gi",
		}))
		Expect(plan.stops).To(ConsistOf(autoGrowStop{
			group: wal, reason: apiv1.ConditionReasonWALArchivingFailing, size: quantity("2Gi"), threshold: 60,
		}))
	})
})

// The life of a volume with autoGrow over the reconciliations: grown by one
// step, waiting for the expansion, stopped at maxSize, resumed when maxSize is
// raised
var _ = Describe("autoGrow reconciliation", func() {
	It("grows the PVCs, records one event per step, and reports the stop at maxSize", func() {
		ctx := context.Background()
		cluster := &apiv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: autoGrowCluster, Namespace: "default", Generation: 1},
			Spec: apiv1.ClusterSpec{
				Instances: 2,
				StorageConfiguration: apiv1.StorageConfiguration{
					Size:     "10Gi",
					AutoGrow: &apiv1.AutoGrowConfiguration{MaxSize: "15Gi", Step: "5Gi"},
				},
			},
		}
		first := autoGrowPVC("1", NewPgDataCalculator(), "10Gi", "10Gi")
		second := autoGrowPVC("2", NewPgDataCalculator(), "10Gi", "10Gi")
		cli := fake.NewClientBuilder().WithScheme(scheme.BuildWithAllKnownScheme()).
			WithObjects(cluster, &first, &second).WithStatusSubresource(cluster).Build()
		recorder := record.NewFakeRecorder(10)

		pvcs := func() []corev1.PersistentVolumeClaim {
			var list corev1.PersistentVolumeClaimList
			Expect(cli.List(ctx, &list)).To(Succeed())
			return list.Items
		}
		requests := func() []string {
			list := pvcs()
			result := make([]string, 0, len(list))
			for _, pvc := range list {
				result = append(result, pvc.Spec.Resources.Requests.Storage().String())
			}
			return result
		}
		// expand completes the expansion requested to the PVCs, as the storage
		// provider does
		expand := func() {
			for _, pvc := range pvcs() {
				pvc.Status.Capacity = corev1.ResourceList{corev1.ResourceStorage: *pvc.Spec.Resources.Requests.Storage()}
				Expect(cli.Status().Update(ctx, &pvc)).To(Succeed())
			}
		}
		stopped := func() *metav1.Condition {
			var stored apiv1.Cluster
			Expect(cli.Get(ctx, client.ObjectKeyFromObject(cluster), &stored)).To(Succeed())
			return meta.FindStatusCondition(stored.Status.Conditions, string(apiv1.ConditionVolumeAutoGrowStopped))
		}

		// The second instance crosses the threshold: both PVCs are grown
		_, err := ReconcileExisting(ctx, cli, recorder, cluster, pvcs(), instancesReporting(
			reporting("1", used("pgdata", 10, 40)),
			reporting("2", used("pgdata", 10, 81)),
		))
		Expect(err).ToNot(HaveOccurred())
		Expect(requests()).To(ConsistOf("15Gi", "15Gi"))
		Expect(recorder.Events).To(Receive(Equal("Normal VolumeAutoGrow Growing the PVCs of the PGDATA volume " +
			"from 10Gi to 15Gi: grow-2 uses 81% of it")))
		Expect(cluster.Spec.StorageConfiguration.Size).To(Equal("10Gi"))

		// The expansion is not done yet: nothing more
		_, err = ReconcileExisting(ctx, cli, recorder, cluster, pvcs(), instancesReporting(
			reporting("2", used("pgdata", 10, 95)),
		))
		Expect(err).ToNot(HaveOccurred())
		Expect(requests()).To(ConsistOf("15Gi", "15Gi"))
		Expect(recorder.Events).ToNot(Receive())

		// Expanded, and still above the threshold: maxSize is reached
		expand()
		_, err = ReconcileExisting(ctx, cli, recorder, cluster, pvcs(), instancesReporting(
			reporting("2", used("pgdata", 15, 90)),
		))
		Expect(err).ToNot(HaveOccurred())
		Expect(requests()).To(ConsistOf("15Gi", "15Gi"))
		condition := stopped()
		Expect(condition.Status).To(Equal(metav1.ConditionTrue))
		Expect(condition.Reason).To(Equal(string(apiv1.ConditionReasonMaxSizeReached)))
		Expect(condition.Message).To(Equal("The PGDATA volume (15Gi) is above its threshold of 80% and is not " +
			"grown: it reached the autoGrow maxSize; raise maxSize, or the size, to grow it again"))
		Expect(recorder.Events).To(Receive(Equal("Warning MaxSizeReached " + condition.Message)))

		// Same stop on the next pass, at another usage: no new event
		_, err = ReconcileExisting(ctx, cli, recorder, cluster, pvcs(), instancesReporting(
			reporting("2", used("pgdata", 15, 97)),
		))
		Expect(err).ToNot(HaveOccurred())
		Expect(recorder.Events).ToNot(Receive())

		// maxSize raised: grown again, and the stop is over
		cluster.Spec.StorageConfiguration.AutoGrow.MaxSize = "30Gi"
		_, err = ReconcileExisting(ctx, cli, recorder, cluster, pvcs(), instancesReporting(
			reporting("2", used("pgdata", 15, 97)),
		))
		Expect(err).ToNot(HaveOccurred())
		Expect(requests()).To(ConsistOf("20Gi", "20Gi"))
		Expect(recorder.Events).To(Receive(HavePrefix("Normal VolumeAutoGrow Growing the PVCs of the PGDATA " +
			"volume from 15Gi to 20Gi")))
		Expect(stopped().Status).To(Equal(metav1.ConditionFalse))
		Expect(recorder.Events).To(Receive(HavePrefix("Normal VolumeAutoGrowResumed ")))
	})
})
