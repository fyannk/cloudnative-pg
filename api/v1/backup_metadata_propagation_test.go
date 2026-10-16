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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/cloudnative-pg/cloudnative-pg/internal/configuration"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/metadatarules"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// A ScheduledBackup puts its rules for the backups on the Backups it
// creates, and a Backup its rules for the volumeSnapshots on its
// VolumeSnapshots, between the default and the mandatory metadata of the
// operator. Without any rule, the Backups are created as before
var _ = Describe("The metadata of the objects of a ScheduledBackup and of a Backup", func() {
	settings := func(defaults, mandatory string) {
		configuration.Current.DefaultMetadata = defaults
		configuration.Current.MandatoryMetadata = mandatory
		Expect(configuration.Current.ParseDefaultAndMandatoryMetadata()).To(Succeed())
	}

	BeforeEach(func() {
		previous := configuration.Current
		configuration.Current = configuration.NewConfiguration()
		DeferCleanup(func() { configuration.Current = previous })
	})

	scheduled := func(annotations map[string]string, rules ...MetadataPropagationRule) *ScheduledBackup {
		return &ScheduledBackup{
			ObjectMeta: metav1.ObjectMeta{Name: "nightly", Namespace: "payments", Annotations: annotations},
			Spec:       ScheduledBackupSpec{MetadataPropagation: rules},
		}
	}

	Context("the Backups of a ScheduledBackup", func() {
		DescribeTable("are created as before without rules",
			func(inheritedAnnotations []string, annotations map[string]string, expected map[string]string) {
				configuration.Current.InheritedAnnotations = inheritedAnnotations
				backup := scheduled(annotations).CreateBackup("nightly-1")
				Expect(backup.Labels).To(BeNil())
				Expect(backup.Annotations).To(Equal(expected))
			},
			Entry("no annotation", nil, nil, map[string]string{}),
			Entry("annotations selected by INHERITED_ANNOTATIONS, and the snapshot deadline",
				[]string{"example.com/*"},
				map[string]string{
					"example.com/owner": "x", "other": "y",
					utils.BackupVolumeSnapshotDeadlineAnnotationName: "20",
				},
				map[string]string{"example.com/owner": "x", utils.BackupVolumeSnapshotDeadlineAnnotationName: "20"}),
		)

		It("get the rules of the ScheduledBackup over its inherited annotations", func() {
			configuration.Current.InheritedAnnotations = []string{"note"}
			backup := scheduled(map[string]string{"note": "inherited"},
				rule(map[string]string{"team": "payments"}, map[string]string{"note": "rule"}, "backups"),
			).CreateBackup("nightly-1")
			Expect(backup.Labels).To(Equal(map[string]string{"team": "payments"}))
			Expect(backup.Annotations).To(Equal(map[string]string{"note": "rule"}))
		})

		It("stack the default metadata, the rules and the mandatory metadata", func() {
			settings(`[{"targets": ["all"], "labels": {"env": "dev", "default": "d"}}]`,
				`[{"targets": ["backups"], "labels": {"env": "prod"}}]`)
			backup := scheduled(nil, rule(map[string]string{"env": "rule", "team": "payments"}, nil, "backups")).
				CreateBackup("nightly-1")
			Expect(backup.Labels).To(Equal(map[string]string{"env": "prod", "default": "d", "team": "payments"}))
		})

		It("get the operator rules naming the ScheduledBackup, not those of the other resources", func() {
			settings(`[{"resources": ["Backup"], "targets": ["all"], "labels": {"snapshot": "d"}}]`,
				`[{"resources": ["ScheduledBackup"], "targets": ["all"], "labels": {"scheduled": "m"}},
				  {"resources": ["Cluster", "Pooler"], "targets": ["all"], "labels": {"cluster": "m"}}]`)
			Expect(scheduled(nil).CreateBackup("nightly-1").Labels).To(Equal(map[string]string{"scheduled": "m"}))
		})

		It("ignore the keys the operator sets itself and the targets of other resources", func() {
			backup := scheduled(map[string]string{utils.BackupVolumeSnapshotDeadlineAnnotationName: "20"},
				rule(map[string]string{"a": "1"}, map[string]string{utils.BackupVolumeSnapshotDeadlineAnnotationName: "1"},
					"backups", "volumeSnapshots"),
			).CreateBackup("nightly-1")
			Expect(backup.Labels).To(Equal(map[string]string{"a": "1"}))
			Expect(backup.Annotations).To(HaveKeyWithValue(utils.BackupVolumeSnapshotDeadlineAnnotationName, "20"))
		})

		It("see what the ScheduledBackup sets on its Backups", func() {
			configuration.Current.InheritedAnnotations = []string{"note"}
			sb := scheduled(map[string]string{"note": "inherited", "other": "x"},
				rule(map[string]string{"team": "payments"}, nil, "backups"))
			Expect(sb.GetScheduledBackupLabelsFor(metadatarules.Backups)).To(Equal(map[string]string{"team": "payments"}))
			Expect(sb.GetScheduledBackupAnnotationsFor(metadatarules.Backups)).
				To(Equal(map[string]string{"note": "inherited"}))
		})
	})

	Context("the VolumeSnapshots of a Backup", func() {
		It("change nothing without rules", func() {
			meta := metav1.ObjectMeta{Labels: map[string]string{"pvc": "data"}}
			(&Backup{}).ApplyMetadataTo(&meta, metadatarules.VolumeSnapshots)
			Expect(meta).To(Equal(metav1.ObjectMeta{Labels: map[string]string{"pvc": "data"}}))
		})

		It("get the operator rules naming the Backup, not those of the other resources", func() {
			settings(`[{"resources": ["Backup"], "targets": ["volumeSnapshots"], "labels": {"snapshot": "d"}}]`,
				`[{"resources": ["ScheduledBackup", "Cluster"], "targets": ["all"], "labels": {"other": "m"}}]`)
			meta := metav1.ObjectMeta{}
			(&Backup{}).ApplyMetadataTo(&meta, metadatarules.VolumeSnapshots)
			Expect(meta.Labels).To(Equal(map[string]string{"snapshot": "d"}))
		})

		It("get the rules of the Backup for the volumeSnapshots only", func() {
			backup := &Backup{Spec: BackupSpec{MetadataPropagation: []MetadataPropagationRule{
				rule(map[string]string{"a": "1"}, map[string]string{"n": "2"}, "volumeSnapshots", "backups"),
			}}}
			Expect(backup.GetBackupLabelsFor(metadatarules.VolumeSnapshots)).To(Equal(map[string]string{"a": "1"}))
			Expect(backup.GetBackupAnnotationsFor(metadatarules.VolumeSnapshots)).To(Equal(map[string]string{"n": "2"}))
			Expect(backup.GetBackupLabelsFor(metadatarules.Backups)).To(BeEmpty())
		})
	})
})
