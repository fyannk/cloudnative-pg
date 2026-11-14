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

package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/tests"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/run"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// SCHEDULED_BACKUP_CRON_FORMAT tells how a five-field schedule is read, see
// "Cron Schedule" in docs/src/backup.md and "Available options" in
// docs/src/operator_conf.md.
//
// No schedule used here comes due while the spec runs, and the scheduled
// backups name a Cluster that does not exist: no backup is ever taken.
var _ = Describe("ScheduledBackup cron format", Serial, Ordered,
	Label(tests.LabelFork, tests.LabelDisruptive, tests.LabelBackupRestore), func() {
		const (
			namespacePrefix = "fork-cron-format"
			level           = tests.Medium

			// read with the seconds field first: the first day of every
			// month at 01:00:00; read as standard cron: the first of January
			// at 00:00
			yearlySchedule = "0 0 1 1 *"
			// read as standard cron: every five minutes
			fiveMinutesSchedule = "*/5 * * * *"

			fieldCountWarning = "Schedule parameter may not have the right number of arguments"
			fieldCountReason  = "ScheduleFieldCount"
		)

		var namespace string

		BeforeEach(func() {
			if testLevelEnv.Depth < int(level) {
				Skip("Test depth is lower than the amount requested for this test")
			}
		})

		BeforeAll(func() {
			if testLevelEnv.Depth < int(level) {
				Skip("Test depth is lower than the amount requested for this test")
			}

			DeferCleanup(saveOperatorConfiguration())

			var err error
			namespace, err = env.CreateUniqueTestNamespace(env.Ctx, env.Client, namespacePrefix)
			Expect(err).ToNot(HaveOccurred())
		})

		scheduledBackup := func(name, schedule string) *apiv1.ScheduledBackup {
			return &apiv1.ScheduledBackup{
				ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name},
				Spec: apiv1.ScheduledBackupSpec{
					Schedule: schedule,
					Cluster:  apiv1.LocalObjectReference{Name: "no-such-cluster"},
				},
			}
		}

		// kubectlCreate creates a ScheduledBackup as a user does, and
		// returns what kubectl prints on its standard error, where the
		// admission warnings are shown
		kubectlCreate := func(name, schedule string) (string, error) {
			manifest := fmt.Sprintf(`apiVersion: postgresql.cnpg.io/v1
kind: ScheduledBackup
metadata:
  name: %s
spec:
  schedule: %q
  cluster:
    name: no-such-cluster
`, name, schedule)
			path := filepath.Join(GinkgoT().TempDir(), name+".yaml")
			Expect(os.WriteFile(path, []byte(manifest), 0o600)).To(Succeed())

			_, stderr, err := run.Unchecked(fmt.Sprintf("kubectl create -n %v -f %v", namespace, path))
			return stderr, err
		}

		// firstScheduleTime returns when the first backup of a ScheduledBackup
		// is due. Its status only holds a next schedule time once a backup
		// has been created: before that, the time is in the BackupSchedule
		// event the ScheduleFieldCount one refers to.
		firstScheduleTime := func(name string) time.Time {
			GinkgoHelper()
			const prefix = "Scheduled first backup by "
			var first time.Time
			Eventually(func(g Gomega) {
				events, err := eventsOf(namespace, "ScheduledBackup", name, "BackupSchedule")
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(events).To(HaveLen(1))
				message := events[0].Message
				g.Expect(message).To(HavePrefix(prefix))
				// the time is printed as "2006-01-02 15:04:05 +0000 UTC"
				fields := strings.Fields(strings.TrimPrefix(message, prefix))
				g.Expect(len(fields)).To(BeNumerically(">=", 3), message)
				first, err = time.Parse("2006-01-02 15:04:05 -0700", strings.Join(fields[:3], " "))
				g.Expect(err).ToNot(HaveOccurred())
			}, 120).Should(Succeed())
			return first.UTC()
		}

		It("reads a five-field schedule with the seconds first by default, and warns about it", func() {
			const name = "five-fields-seconds"

			By("showing an admission warning", func() {
				stderr, err := kubectlCreate(name, yearlySchedule)
				Expect(err).ToNot(HaveOccurred())
				Expect(stderr).To(ContainSubstring(fieldCountWarning))
			})

			By("recording a ScheduleFieldCount Warning event on the ScheduledBackup", func() {
				Eventually(func(g Gomega) {
					events, err := eventsOf(namespace, "ScheduledBackup", name, fieldCountReason)
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(events).ToNot(BeEmpty())
					g.Expect(events[0].Type).To(Equal(corev1.EventTypeWarning))
					g.Expect(events[0].Message).To(ContainSubstring(yearlySchedule))
				}, 120).Should(Succeed())
			})

			By("scheduling the next backup the first day of a month at 01:00:00", func() {
				next := firstScheduleTime(name)
				Expect(next.Day()).To(Equal(1), next.String())
				Expect(next.Hour()).To(Equal(1), next.String())
				Expect(next.Minute()).To(Equal(0), next.String())
				Expect(next.Second()).To(Equal(0), next.String())
				// within a month, while standard cron would wait for January
				Expect(next).To(BeTemporally("<", time.Now().Add(32*24*time.Hour)))
			})

			By("saying nothing about a schedule descriptor", func() {
				stderr, err := kubectlCreate("descriptor", "@yearly")
				Expect(err).ToNot(HaveOccurred())
				Expect(stderr).ToNot(ContainSubstring(fieldCountWarning))
			})
		})

		It("reads a five-field schedule as standard cron with the standard format", func() {
			const name = "five-fields-standard"

			setOperatorConfiguration(map[string]string{"SCHEDULED_BACKUP_CRON_FORMAT": "standard"})

			// keep clear of the next multiple of five minutes, when the
			// schedule comes due and the next one is computed
			if untilDue := time.Until(time.Now().Truncate(5 * time.Minute).Add(5 * time.Minute)); untilDue < 30*time.Second {
				time.Sleep(untilDue + 5*time.Second)
			}

			By("creating the ScheduledBackup without a warning", func() {
				stderr, err := kubectlCreate(name, fiveMinutesSchedule)
				Expect(err).ToNot(HaveOccurred())
				Expect(stderr).ToNot(ContainSubstring(fieldCountWarning))
			})

			By("scheduling the next backup at the next multiple of five minutes", func() {
				next := firstScheduleTime(name)
				Expect(next.Second()).To(Equal(0), next.String())
				Expect(next.Minute()%5).To(Equal(0), next.String())
				Expect(next).To(BeTemporally("~", time.Now(), 5*time.Minute+10*time.Second))
			})

			By("recording no ScheduleFieldCount event", func() {
				events, err := eventsOf(namespace, "ScheduledBackup", name, fieldCountReason)
				Expect(err).ToNot(HaveOccurred())
				Expect(events).To(BeEmpty())
			})

			By("deleting the ScheduledBackup before its schedule comes due", func() {
				Expect(env.Client.Delete(env.Ctx, scheduledBackup(name, fiveMinutesSchedule))).To(Succeed())
			})
		})

		It("refuses the creation of a five-field schedule with the seconds-strict format", func() {
			setOperatorConfiguration(map[string]string{"SCHEDULED_BACKUP_CRON_FORMAT": "seconds-strict"})

			By("refusing a five-field schedule, with its six-field equivalent in the message", func() {
				err := env.Client.Create(env.Ctx, scheduledBackup("five-fields-strict", fiveMinutesSchedule))
				Expect(apierrors.IsInvalid(err)).To(BeTrue(), "%v", err)
				Expect(err.Error()).To(ContainSubstring("spec.schedule"))
				Expect(err.Error()).To(ContainSubstring(`"0 */5 * * * *"`))
			})

			By("accepting a six-field schedule", func() {
				Expect(env.Client.Create(env.Ctx, scheduledBackup("six-fields-strict", "0 0 0 1 1 *"))).To(Succeed())
			})

			By("still accepting the update of an existing five-field schedule", func() {
				err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
					current := &apiv1.ScheduledBackup{}
					if err := env.Client.Get(env.Ctx,
						ctrlclient.ObjectKey{Namespace: namespace, Name: "five-fields-seconds"}, current); err != nil {
						return err
					}
					Expect(current.Spec.Schedule).To(Equal(yearlySchedule))
					current.Spec.Suspend = new(true)
					return env.Client.Update(env.Ctx, current)
				})
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})
