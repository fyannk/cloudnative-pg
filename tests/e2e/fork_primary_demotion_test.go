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
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/cloudnative-pg/cloudnative-pg/tests"
	clusterasserts "github.com/cloudnative-pg/cloudnative-pg/tests/internal/asserts/cluster"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/clusterutils"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/exec"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/run"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/timeouts"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// A primary that is no longer the target primary of its cluster does not
// wait for its sessions to end, see "Shutdown control" and "Shutdown of the
// primary during a switchover" in docs/src/instance_manager.md.
var _ = Describe("Primary demotion with an open session", Ordered,
	Label(tests.LabelFork, tests.LabelSelfHealing), func() {
		const (
			clusterFile     = forkFixturesDir + "/cluster-demotion.yaml.template"
			clusterName     = "fork-demotion"
			namespacePrefix = "fork-demotion"
			level           = tests.Medium

			// the smartShutdownTimeout of the fixture: what a smart shutdown
			// waits for a session that never ends
			smartShutdownTimeout = 180 * time.Second
			// a demotion taking less than this did not wait for the session
			demotionDeadline = smartShutdownTimeout / 2
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

			var err error
			namespace, err = env.CreateUniqueTestNamespace(env.Ctx, env.Client, namespacePrefix)
			Expect(err).ToNot(HaveOccurred())
			clusterasserts.AssertCreateCluster(env, testTimeouts, namespace, clusterName, clusterFile)
		})

		// openIdleSession opens a session on the instance and leaves it
		// idle, as a connection pool does. The session lasts as long as the
		// PostgreSQL container: nothing closes it from the client side.
		openIdleSession := func(podName, applicationName string) {
			GinkgoHelper()

			go func() {
				defer GinkgoRecover()
				// psql waits for its commands on the output of sleep
				_, _, _ = run.Unchecked(fmt.Sprintf(
					`kubectl exec -n %v %v -c postgres -- `+
						`sh -c "sleep 3600 | PGAPPNAME=%v psql -X -U postgres postgres"`,
					namespace, podName, applicationName))
			}()

			Eventually(func(g Gomega) {
				out, _, err := exec.QueryInInstancePod(
					env.Ctx, env.Client, env.Interface, env.RestClientConfig,
					exec.PodLocator{Namespace: namespace, PodName: podName},
					"postgres",
					fmt.Sprintf("SELECT count(*) FROM pg_catalog.pg_stat_activity "+
						"WHERE application_name = '%v' AND state = 'idle'", applicationName))
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(strings.TrimSpace(out)).To(Equal("1"))
			}, 60).Should(Succeed())
		}

		currentPrimary := func() (string, error) {
			cluster, err := clusterutils.Get(env.Ctx, env.Client, namespace, clusterName)
			if err != nil {
				return "", err
			}
			return cluster.Status.CurrentPrimary, nil
		}

		It("does not wait for the session during a switchover", func() {
			primary, err := currentPrimary()
			Expect(err).ToNot(HaveOccurred())
			openIdleSession(primary, "fork-e2e-switchover")

			// the new primary is expected within NewPrimaryAfterSwitchover,
			// far below the time a smart shutdown would wait for the session
			Expect(testTimeouts[timeouts.NewPrimaryAfterSwitchover]).To(
				BeNumerically("<", demotionDeadline.Seconds()))
			clusterasserts.AssertSwitchover(env, testTimeouts, namespace, clusterName)
		})

		It("escalates the smart shutdown of a deleted primary Pod to a fast one", func() {
			primary, err := currentPrimary()
			Expect(err).ToNot(HaveOccurred())
			primaryPod, err := clusterutils.GetPrimary(env.Ctx, env.Client, namespace, clusterName)
			Expect(err).ToNot(HaveOccurred())
			Expect(primaryPod.Name).To(Equal(primary))

			openIdleSession(primary, "fork-e2e-deletion")

			// the logs of the instance manager, until its container stops
			primaryLogs := make(chan string, 1)
			go func() {
				defer GinkgoRecover()
				stdout, _, _ := run.Unchecked(fmt.Sprintf(
					"kubectl logs -f --tail=-1 -n %v %v -c postgres", namespace, primary))
				primaryLogs <- stdout
			}()

			By("deleting the Pod of the primary, without a prior switchover", func() {
				// with the grace period of the Pod, its stopDelay: the kubelet
				// leaves the shutdown to the instance manager
				Expect(env.Client.Delete(env.Ctx, primaryPod)).To(Succeed())
			})
			deletedAt := time.Now()

			By("promoting the other instance without waiting for the session", func() {
				Eventually(currentPrimary, demotionDeadline, time.Second).ShouldNot(Equal(primary))
			})

			By("stopping the former primary without waiting for the session", func() {
				var logs string
				Eventually(primaryLogs, demotionDeadline, time.Second).Should(Receive(&logs))
				Expect(time.Since(deletedAt)).To(BeNumerically("<", demotionDeadline))

				// "the smart shutdown is skipped, or escalated to a fast one as
				// soon as the operator takes that decision"
				Expect(logs).To(Or(
					ContainSubstring("escalating the smart shutdown in progress to a fast shutdown"),
					ContainSubstring("skipping the smart shutdown"),
				))
				Expect(logs).To(ContainSubstring("PostgreSQL instance shut down"))
			})

			By("having the former primary back as a replica", func() {
				clusterasserts.AssertClusterIsReady(env, namespace, clusterName, testTimeouts[timeouts.ClusterIsReady])

				Eventually(func(g Gomega) {
					podList, err := clusterutils.ListPods(env.Ctx, env.Client, namespace, clusterName)
					g.Expect(err).ToNot(HaveOccurred())
					var formerPrimary *corev1.Pod
					for idx := range podList.Items {
						if podList.Items[idx].Name == primary {
							formerPrimary = &podList.Items[idx]
						}
					}
					g.Expect(formerPrimary).ToNot(BeNil())
					g.Expect(formerPrimary.UID).ToNot(Equal(primaryPod.UID))
					g.Expect(formerPrimary.Labels).To(HaveKeyWithValue("cnpg.io/instanceRole", "replica"))
				}, 60).Should(Succeed())
			})
		})
	})
