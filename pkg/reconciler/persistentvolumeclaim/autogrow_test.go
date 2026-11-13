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
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

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

// crashTime is the time a PostgreSQL container terminated in the tests, and
// its identity as recorded on the PVCs
var crashTime = metav1.NewTime(time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC))

const crashID = "2026-11-13T10:00:00Z"

// outOfWALSpace is the status of the first instance, whose PostgreSQL
// stopped because the WAL volume is full: it cannot report its usage
func outOfWALSpace() postgres.PostgresqlStatus {
	return outOfSpaceAt(crashTime)
}

// outOfSpaceAt is the status of the first instance, whose PostgreSQL
// container terminated at the given time for a lack of disk space, and was
// restarted by the kubelet without being ready again
func outOfSpaceAt(finishedAt metav1.Time) postgres.PostgresqlStatus {
	status := reporting("1")
	status.Pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name:  specs.PostgresContainerName,
		State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
		LastTerminationState: corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{
				ExitCode:   apiv1.MissingWALDiskSpaceExitCode,
				FinishedAt: finishedAt,
			},
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

	It("does not step again while a PVC of the volume is below the size of the previous step", func() {
		// the expansion of the second PVC to 12Gi was refused: it still
		// requests 10Gi, with nothing in progress
		pvcs := []corev1.PersistentVolumeClaim{
			autoGrowPVC("1", NewPgDataCalculator(), "12Gi", "12Gi"),
			autoGrowPVC("2", NewPgDataCalculator(), "10Gi", "10Gi"),
		}

		plan, err := planAutoGrow(cluster, pvcs, instancesReporting(
			reporting("1", used("pgdata", 12, 95)),
			reporting("2", used("pgdata", 10, 95)),
		))

		Expect(err).ToNot(HaveOccurred())
		// the same step is requested again to the refused PVC
		Expect(sizesOf(plan)).To(Equal(map[pvcGroup]string{pgData: "12Gi"}))
		Expect(plan.steps).To(BeEmpty())
		Expect(plan.stops).To(BeEmpty())
	})

	It("ignores a filesystem that is not the one of the PVC", func() {
		// local-path: the volume is a directory of the node filesystem
		plan, err := planAutoGrow(cluster, dataPVCs("10Gi"), instancesReporting(reporting("1", used("pgdata", 500, 95))))

		Expect(err).ToNot(HaveOccurred())
		Expect(plan.steps).To(BeEmpty())
		Expect(plan.stops).To(BeEmpty())
	})

	It("grows the volume holding the WAL files of an instance stopped for lack of WAL space", func() {
		plan, err := planAutoGrow(cluster, dataPVCs("10Gi"), instancesReporting(outOfWALSpace()))

		Expect(err).ToNot(HaveOccurred())
		Expect(sizesOf(plan)[pgData]).To(Equal("12Gi"))
		Expect(plan.steps).To(HaveLen(1))
		Expect(plan.steps[0].usedPercent).To(BeEquivalentTo(100))
		Expect(plan.steps[0].diskFull).To(BeTrue())
		Expect(plan.terminations).To(ConsistOf(autoGrowTermination{
			group: pgData, pvcName: autoGrowCluster + "-1", finishedAt: crashID,
		}))
	})

	It("grows the volume once per termination for a full volume", func() {
		pvcs := dataPVCs("10Gi")
		pvcs[0].Annotations[utils.AutoGrowLastTerminationAnnotationName] = crashID

		// the termination recorded on the PVC of the instance was handled
		plan, err := planAutoGrow(cluster, pvcs, instancesReporting(outOfWALSpace()))
		Expect(err).ToNot(HaveOccurred())
		Expect(sizesOf(plan)[pgData]).To(Equal("10Gi"))
		Expect(plan.steps).To(BeEmpty())
		Expect(plan.terminations).To(BeEmpty())

		// a new termination of the same instance grows it again
		plan, err = planAutoGrow(cluster, pvcs, instancesReporting(
			outOfSpaceAt(metav1.NewTime(crashTime.Add(5*time.Minute)))))
		Expect(err).ToNot(HaveOccurred())
		Expect(sizesOf(plan)[pgData]).To(Equal("12Gi"))
		Expect(plan.terminations).To(ConsistOf(autoGrowTermination{
			group: pgData, pvcName: autoGrowCluster + "-1", finishedAt: "2026-11-13T10:05:00Z",
		}))
	})

	It("grows only the volumes named full by the instance manager", func() {
		cluster.Spec.WalStorage = &apiv1.StorageConfiguration{
			Size:     "2Gi",
			AutoGrow: &apiv1.AutoGrowConfiguration{MaxSize: "4Gi", Step: "1Gi"},
		}
		cluster.Spec.Tablespaces = []apiv1.TablespaceConfiguration{{
			Name: "alpha",
			Storage: apiv1.StorageConfiguration{
				Size:     "5Gi",
				AutoGrow: &apiv1.AutoGrowConfiguration{MaxSize: "50Gi", Step: "5Gi"},
			},
		}}
		pvcs := append(dataPVCs("10Gi"),
			autoGrowPVC("1", NewPgWalCalculator(), "2Gi", "2Gi"),
			autoGrowPVC("1", NewPgTablespaceCalculator("alpha"), "5Gi", "5Gi"),
		)
		withMessage := func(message string) postgres.PostgresqlStatus {
			status := outOfWALSpace()
			status.Pod.Status.ContainerStatuses[0].LastTerminationState.Terminated.Message = message
			return status
		}

		// the recovery of PostgreSQL fails on the full tablespace
		plan, err := planAutoGrow(cluster, pvcs, instancesReporting(
			withMessage(`{"reason":"DiskFull","volumes":["tbs-alpha"]}`)))
		Expect(err).ToNot(HaveOccurred())
		Expect(sizesOf(plan)).To(Equal(map[pvcGroup]string{pgData: "10Gi", wal: "2Gi", tablespace: "10Gi"}))
		Expect(plan.steps).To(HaveLen(1))
		Expect(plan.steps[0].diskFull).To(BeTrue())
		Expect(plan.terminations).To(ConsistOf(autoGrowTermination{
			group: tablespace, pvcName: autoGrowCluster + "-1-tbs-alpha", finishedAt: crashID,
		}))

		// PGDATA and the tablespace are full
		plan, err = planAutoGrow(cluster, pvcs, instancesReporting(
			withMessage(`{"reason":"DiskFull","volumes":["pgdata","tbs-alpha"]}`)))
		Expect(err).ToNot(HaveOccurred())
		Expect(sizesOf(plan)).To(Equal(map[pvcGroup]string{pgData: "12Gi", wal: "2Gi", tablespace: "10Gi"}))

		// an older instance manager names no volume: the WAL volume is full
		for _, message := range []string{"", "not a termination message"} {
			plan, err = planAutoGrow(cluster, pvcs, instancesReporting(withMessage(message)))
			Expect(err).ToNot(HaveOccurred())
			Expect(sizesOf(plan)).To(Equal(map[pvcGroup]string{pgData: "10Gi", wal: "3Gi", tablespace: "5Gi"}))
		}
	})

	It("attributes a termination for a full volume to the expansion in progress", func() {
		pvcs := dataPVCs("12Gi")
		pvcs[0].Status.Capacity[corev1.ResourceStorage] = quantity("10Gi")

		plan, err := planAutoGrow(cluster, pvcs, instancesReporting(outOfWALSpace()))

		Expect(err).ToNot(HaveOccurred())
		Expect(sizesOf(plan)[pgData]).To(Equal("12Gi"))
		Expect(plan.steps).To(BeEmpty())
		Expect(plan.terminations).To(ConsistOf(autoGrowTermination{
			group: pgData, pvcName: autoGrowCluster + "-1", finishedAt: crashID,
		}))
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

// A step refused to one PVC of the volume: the other PVCs must not be grown
// again from the size they accepted, while the refused one stays behind
var _ = Describe("autoGrow with a refused expansion", func() {
	It("retries the same step on the refused PVC, and steps again only once every PVC got it", func() {
		ctx := context.Background()
		cluster := &apiv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: autoGrowCluster, Namespace: "default", Generation: 1},
			Spec: apiv1.ClusterSpec{
				Instances: 2,
				StorageConfiguration: apiv1.StorageConfiguration{
					Size:     "10Gi",
					AutoGrow: &apiv1.AutoGrowConfiguration{MaxSize: "100Gi", Step: "5Gi"},
				},
			},
		}
		first := autoGrowPVC("1", NewPgDataCalculator(), "10Gi", "10Gi")
		second := autoGrowPVC("2", NewPgDataCalculator(), "10Gi", "10Gi")
		base := fake.NewClientBuilder().WithScheme(scheme.BuildWithAllKnownScheme()).
			WithObjects(cluster, &first, &second).WithStatusSubresource(cluster).Build()
		refuse := true
		cli := interceptor.NewClient(base, interceptor.Funcs{
			Patch: func(ctx context.Context, c client.WithWatch, obj client.Object,
				patch client.Patch, opts ...client.PatchOption,
			) error {
				if refuse && obj.GetName() == second.Name {
					return forbiddenPVC(obj.GetName(), quotaRefusal)
				}
				return c.Patch(ctx, obj, patch, opts...)
			},
		})
		recorder := record.NewFakeRecorder(10)

		pvcs := func() []corev1.PersistentVolumeClaim {
			var list corev1.PersistentVolumeClaimList
			Expect(cli.List(ctx, &list)).To(Succeed())
			return list.Items
		}
		requests := func() map[string]string {
			result := make(map[string]string)
			for _, pvc := range pvcs() {
				result[pvc.Name] = pvc.Spec.Resources.Requests.Storage().String()
			}
			return result
		}
		expand := func() {
			for _, pvc := range pvcs() {
				pvc.Status.Capacity = corev1.ResourceList{corev1.ResourceStorage: *pvc.Spec.Resources.Requests.Storage()}
				Expect(cli.Status().Update(ctx, &pvc)).To(Succeed())
			}
		}
		full := instancesReporting(
			reporting("1", used("pgdata", 15, 95)),
			reporting("2", used("pgdata", 10, 95)),
		)

		_, err := ReconcileExisting(ctx, cli, recorder, cluster, pvcs(), instancesReporting(
			reporting("1", used("pgdata", 10, 95)),
			reporting("2", used("pgdata", 10, 95)),
		))
		Expect(err).ToNot(HaveOccurred())
		Expect(requests()).To(Equal(map[string]string{first.Name: "15Gi", second.Name: "10Gi"}))

		// the first PVC got its step, the second is still refused: the
		// same step is retried, and the first PVC is not grown again
		for range 3 {
			expand()
			_, err = ReconcileExisting(ctx, cli, recorder, cluster, pvcs(), full)
			Expect(err).ToNot(HaveOccurred())
			Expect(requests()).To(Equal(map[string]string{first.Name: "15Gi", second.Name: "10Gi"}))
		}

		// the quota is raised: the second PVC gets the step, and only
		// then the volume is grown again
		refuse = false
		_, err = ReconcileExisting(ctx, cli, recorder, cluster, pvcs(), full)
		Expect(err).ToNot(HaveOccurred())
		Expect(requests()).To(Equal(map[string]string{first.Name: "15Gi", second.Name: "15Gi"}))

		expand()
		_, err = ReconcileExisting(ctx, cli, recorder, cluster, pvcs(), instancesReporting(
			reporting("1", used("pgdata", 15, 95)),
			reporting("2", used("pgdata", 15, 95)),
		))
		Expect(err).ToNot(HaveOccurred())
		Expect(requests()).To(Equal(map[string]string{first.Name: "20Gi", second.Name: "20Gi"}))
	})
})

// An instance whose PostgreSQL stopped because the volume is full stays so
// until the kubelet restarts it, up to five minutes in CrashLoopBackOff,
// while the cluster is reconciled every ten seconds
var _ = Describe("autoGrow for a full volume", func() {
	It("grows the volume once per termination of PostgreSQL", func() {
		ctx := context.Background()
		cluster := &apiv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: autoGrowCluster, Namespace: "default", Generation: 1},
			Spec: apiv1.ClusterSpec{
				Instances: 2,
				StorageConfiguration: apiv1.StorageConfiguration{
					Size:     "10Gi",
					AutoGrow: &apiv1.AutoGrowConfiguration{MaxSize: "100Gi", Step: "5Gi"},
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
		expand := func() {
			for _, pvc := range pvcs() {
				pvc.Status.Capacity = corev1.ResourceList{corev1.ResourceStorage: *pvc.Spec.Resources.Requests.Storage()}
				Expect(cli.Status().Update(ctx, &pvc)).To(Succeed())
			}
		}
		crashed := instancesReporting(outOfWALSpace(), reporting("2", used("pgdata", 10, 40)))

		_, err := ReconcileExisting(ctx, cli, recorder, cluster, pvcs(), crashed)
		Expect(err).ToNot(HaveOccurred())
		Expect(requests()).To(ConsistOf("15Gi", "15Gi"))
		Expect(recorder.Events).To(Receive(Equal("Normal VolumeAutoGrow Growing the PVCs of the PGDATA volume " +
			"from 10Gi to 15Gi: PostgreSQL of grow-1 stopped because the volume is full")))
		var stored corev1.PersistentVolumeClaim
		Expect(cli.Get(ctx, client.ObjectKeyFromObject(&first), &stored)).To(Succeed())
		Expect(stored.Annotations).To(HaveKeyWithValue(utils.AutoGrowLastTerminationAnnotationName, crashID))

		// expanded, and the container is still waiting to be restarted
		// after the same termination: nothing more
		expand()
		for range 3 {
			_, err = ReconcileExisting(ctx, cli, recorder, cluster, pvcs(), crashed)
			Expect(err).ToNot(HaveOccurred())
			Expect(requests()).To(ConsistOf("15Gi", "15Gi"))
			Expect(recorder.Events).ToNot(Receive())
		}

		// PostgreSQL stopped again, the volume is still too small
		_, err = ReconcileExisting(ctx, cli, recorder, cluster, pvcs(), instancesReporting(
			outOfSpaceAt(metav1.NewTime(crashTime.Add(5*time.Minute)))))
		Expect(err).ToNot(HaveOccurred())
		Expect(requests()).To(ConsistOf("20Gi", "20Gi"))
		Expect(recorder.Events).To(Receive(HavePrefix("Normal VolumeAutoGrow Growing the PVCs of the PGDATA " +
			"volume from 15Gi to 20Gi")))
	})

	It("does not record a termination while the expansion of its volume is refused", func() {
		ctx := context.Background()
		first := autoGrowPVC("1", NewPgDataCalculator(), "10Gi", "10Gi")
		second := autoGrowPVC("2", NewPgDataCalculator(), "10Gi", "10Gi")
		cli := fake.NewClientBuilder().WithScheme(scheme.BuildWithAllKnownScheme()).
			WithObjects(&first, &second).Build()
		pvcs := []corev1.PersistentVolumeClaim{first, second}
		pgData := pvcGroup{role: utils.PVCRolePgData}
		terminations := []autoGrowTermination{{group: pgData, pvcName: first.Name, finishedAt: crashID}}
		refusals := []*volumeExpansionRefusal{newVolumeExpansionRefusal(&second, quantity("15Gi"),
			forbiddenPVC(second.Name, quotaRefusal))}

		Expect(recordAutoGrowTerminations(ctx, cli, pvcs, terminations, refusals)).To(Succeed())
		var stored corev1.PersistentVolumeClaim
		Expect(cli.Get(ctx, client.ObjectKeyFromObject(&first), &stored)).To(Succeed())
		Expect(stored.Annotations).ToNot(HaveKey(utils.AutoGrowLastTerminationAnnotationName))

		Expect(recordAutoGrowTerminations(ctx, cli, pvcs, terminations, nil)).To(Succeed())
		Expect(cli.Get(ctx, client.ObjectKeyFromObject(&first), &stored)).To(Succeed())
		Expect(stored.Annotations).To(HaveKeyWithValue(utils.AutoGrowLastTerminationAnnotationName, crashID))
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
