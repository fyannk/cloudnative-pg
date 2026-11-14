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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/internal/scheme"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// A storage class that cannot resize a volume in use leaves the filesystem
// as it is until the Pod restarts: upstream documents the restart, and the
// operator now says which instances wait for it
var _ = Describe("filesystem resize waiting for a restart of the Pod", func() {
	clock := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)

	pvcOf := func(instance string, pendingSince *time.Time) corev1.PersistentVolumeClaim {
		pvc := corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: instance, Namespace: "default",
				Labels: map[string]string{utils.InstanceNameLabelName: instance},
			},
		}
		if pendingSince != nil {
			pvc.Status.Conditions = []corev1.PersistentVolumeClaimCondition{{
				Type:               corev1.PersistentVolumeClaimFileSystemResizePending,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: metav1.NewTime(*pendingSince),
			}}
		}
		return pvc
	}
	ago := func(d time.Duration) *time.Time {
		t := clock.Add(-d)
		return &t
	}

	BeforeEach(func() {
		previous := now
		now = func() time.Time { return clock }
		DeferCleanup(func() { now = previous })
	})

	It("waits for the grace period before asking for a restart", func() {
		pvcs := []corev1.PersistentVolumeClaim{pvcOf("cluster-1", ago(30*time.Second)), pvcOf("cluster-2", nil)}
		Expect(IsFileSystemResizeAwaited(pvcs)).To(BeTrue())
		Expect(getInstancesToRestartForResize(pvcs)).To(BeEmpty())

		pvcs = []corev1.PersistentVolumeClaim{
			pvcOf("cluster-2", ago(10*time.Minute)), pvcOf("cluster-1", ago(3*time.Minute)), pvcOf("cluster-3", nil),
		}
		Expect(IsFileSystemResizeAwaited(pvcs)).To(BeFalse())
		Expect(getInstancesToRestartForResize(pvcs)).To(Equal([]string{"cluster-1", "cluster-2"}))
	})

	It("reports the instances to restart, once, and the end of the wait", func(ctx context.Context) {
		cluster := &apiv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster", Namespace: "default"}}
		cli := fake.NewClientBuilder().WithScheme(scheme.BuildWithAllKnownScheme()).
			WithObjects(cluster).WithStatusSubresource(cluster).Build()
		recorder := record.NewFakeRecorder(10)

		By("setting no condition on a cluster that never waited")
		Expect(reportPendingFileSystemResize(ctx, cli, recorder, cluster,
			[]corev1.PersistentVolumeClaim{pvcOf("cluster-1", nil)})).To(Succeed())
		Expect(meta.FindStatusCondition(cluster.Status.Conditions,
			string(apiv1.ConditionVolumeResizePending))).To(BeNil())

		By("naming the instances whose filesystem waits")
		pending := []corev1.PersistentVolumeClaim{pvcOf("cluster-1", ago(5*time.Minute))}
		Expect(reportPendingFileSystemResize(ctx, cli, recorder, cluster, pending)).To(Succeed())
		condition := meta.FindStatusCondition(cluster.Status.Conditions, string(apiv1.ConditionVolumeResizePending))
		Expect(condition).ToNot(BeNil())
		Expect(condition.Status).To(Equal(metav1.ConditionTrue))
		Expect(condition.Message).To(ContainSubstring("cluster-1"))
		Expect(<-recorder.Events).To(HavePrefix("Warning FileSystemResizePending "))

		By("recording no other event while nothing changes")
		Expect(reportPendingFileSystemResize(ctx, cli, recorder, cluster, pending)).To(Succeed())
		Expect(recorder.Events).ToNot(Receive())

		By("reporting the end of the wait")
		Expect(reportPendingFileSystemResize(ctx, cli, recorder, cluster,
			[]corev1.PersistentVolumeClaim{pvcOf("cluster-1", nil)})).To(Succeed())
		condition = meta.FindStatusCondition(cluster.Status.Conditions, string(apiv1.ConditionVolumeResizePending))
		Expect(condition.Status).To(Equal(metav1.ConditionFalse))
		Expect(<-recorder.Events).To(HavePrefix("Normal VolumeResizeCompleted "))
	})
})
