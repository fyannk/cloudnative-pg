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
	"time"

	corev1 "k8s.io/api/core/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"
	"github.com/cloudnative-pg/cloudnative-pg/tests"
	clusterasserts "github.com/cloudnative-pg/cloudnative-pg/tests/internal/asserts/cluster"
	"github.com/cloudnative-pg/cloudnative-pg/tests/internal/resources"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/clusterutils"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/timeouts"
	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/yaml"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The labels and annotations per kind of object: the `metadataPropagation`
// rules of a Cluster, the DEFAULT_METADATA and MANDATORY_METADATA settings of
// the operator, and the monitoring discovery annotations. See "Labels and
// annotations per kind of object" and "Default and mandatory labels and
// annotations" in docs/src/labels_annotations.md, and "Collecting metrics and
// logs without the Prometheus Operator" in docs/src/monitoring.md.
var _ = Describe("Metadata propagation rules", Serial, Ordered,
	Label(tests.LabelFork, tests.LabelDisruptive, tests.LabelClusterMetadata), func() {
		const (
			clusterFile     = forkFixturesDir + "/cluster-metadata.yaml.template"
			namespacePrefix = "fork-metadata"
			level           = tests.Medium

			mandatoryLabel  = "fork-e2e/mandatory"
			defaultLabel    = "fork-e2e/default"
			overriddenLabel = "fork-e2e/overridden"
			ruleKey         = "fork-e2e/rule"
			scrapeKey       = "prometheus.io/scrape"

			mandatoryMetadata = `
- targets: [all]
  labels:
    fork-e2e/mandatory: enforced
`
			defaultMetadata = `
- targets: [pgPods]
  labels:
    fork-e2e/default: fallback
    fork-e2e/overridden: from-default
`
		)

		var (
			namespace, clusterName   string
			initdbJobPod, joinJobPod corev1.Pod
		)

		BeforeEach(func() {
			if testLevelEnv.Depth < int(level) {
				Skip("Test depth is lower than the amount requested for this test")
			}
		})

		// jobPod waits for the Pod of the Job of the given role, which lasts
		// as long as the Job does, and returns it
		jobPod := func(role string) corev1.Pod {
			GinkgoHelper()
			var result corev1.Pod
			Eventually(func(g Gomega) {
				podList := &corev1.PodList{}
				g.Expect(env.Client.List(env.Ctx, podList, ctrlclient.InNamespace(namespace),
					ctrlclient.MatchingLabels{utils.JobRoleLabelName: role})).To(Succeed())
				g.Expect(podList.Items).ToNot(BeEmpty())
				result = podList.Items[0]
			}, testTimeouts[timeouts.ClusterIsReady], 500*time.Millisecond).Should(Succeed())
			return result
		}

		BeforeAll(func() {
			if testLevelEnv.Depth < int(level) {
				Skip("Test depth is lower than the amount requested for this test")
			}

			DeferCleanup(saveOperatorConfiguration())
			setOperatorConfiguration(map[string]string{
				"MANDATORY_METADATA":               mandatoryMetadata,
				"DEFAULT_METADATA":                 defaultMetadata,
				"MONITORING_DISCOVERY_ANNOTATIONS": "prometheus",
			})

			var err error
			namespace, err = env.CreateUniqueTestNamespace(env.Ctx, env.Client, namespacePrefix)
			Expect(err).ToNot(HaveOccurred())
			clusterName, err = yaml.GetResourceNameFromYAML(env.Scheme, clusterFile)
			Expect(err).ToNot(HaveOccurred())

			// the Pods of the Jobs are gone once the cluster is ready: they
			// are collected while it is being created
			resources.CreateResourceFromFile(env, namespace, clusterFile)
			initdbJobPod = jobPod("initdb")
			joinJobPod = jobPod("join")
			clusterasserts.AssertClusterIsReady(env, namespace, clusterName, testTimeouts[timeouts.ClusterIsReady])
		})

		It("puts the mandatory label and the rules for pgPods on the Pods of the bootstrap and join Jobs", func() {
			for _, pod := range []corev1.Pod{initdbJobPod, joinJobPod} {
				By("checking the Pod " + pod.Name)
				Expect(pod.Labels).To(HaveKeyWithValue(mandatoryLabel, "enforced"))
				Expect(pod.Labels).To(HaveKeyWithValue(ruleKey, "pods"))
			}
		})

		It("puts the mandatory, default and rule labels on the instance Pods", func() {
			Eventually(func(g Gomega) {
				podList, err := clusterutils.ListPods(env.Ctx, env.Client, namespace, clusterName)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(podList.Items).To(HaveLen(2))
				for _, pod := range podList.Items {
					// MANDATORY_METADATA wins over inheritedMetadata
					g.Expect(pod.Labels).To(HaveKeyWithValue(mandatoryLabel, "enforced"), pod.Name)
					// DEFAULT_METADATA applies when the Cluster does not set the key
					g.Expect(pod.Labels).To(HaveKeyWithValue(defaultLabel, "fallback"), pod.Name)
					// and is replaced when it does
					g.Expect(pod.Labels).To(HaveKeyWithValue(overriddenLabel, "from-cluster"), pod.Name)
					g.Expect(pod.Labels).To(HaveKeyWithValue(ruleKey, "pods"), pod.Name)
				}
			}, 120).Should(Succeed())
		})

		It("never takes a label of the operator from a rule", func() {
			podList, err := clusterutils.ListPods(env.Ctx, env.Client, namespace, clusterName)
			Expect(err).ToNot(HaveOccurred())
			for _, pod := range podList.Items {
				Expect(pod.Labels[utils.ClusterInstanceRoleLabelName]).To(
					BeElementOf("primary", "replica"), pod.Name)
			}
			for _, pod := range []corev1.Pod{initdbJobPod, joinJobPod} {
				Expect(pod.Labels).ToNot(HaveKeyWithValue(utils.ClusterInstanceRoleLabelName, "from-rule"), pod.Name)
			}
		})

		It("puts the mandatory label and the rules on the Services and on the PVCs", func() {
			Eventually(func(g Gomega) {
				for _, suffix := range []string{"-rw", "-ro", "-r"} {
					service := &corev1.Service{}
					g.Expect(env.Client.Get(env.Ctx,
						ctrlclient.ObjectKey{Namespace: namespace, Name: clusterName + suffix}, service)).To(Succeed())
					g.Expect(service.Labels).To(HaveKeyWithValue(mandatoryLabel, "enforced"), service.Name)
					g.Expect(service.Labels).To(HaveKeyWithValue(ruleKey, "services"), service.Name)
					// a rule for another target does not reach the Services
					g.Expect(service.Labels).ToNot(HaveKey(defaultLabel), service.Name)
				}

				pvcList := &corev1.PersistentVolumeClaimList{}
				g.Expect(env.Client.List(env.Ctx, pvcList, ctrlclient.InNamespace(namespace),
					ctrlclient.MatchingLabels{utils.ClusterLabelName: clusterName})).To(Succeed())
				g.Expect(pvcList.Items).To(HaveLen(2))
				for _, pvc := range pvcList.Items {
					g.Expect(pvc.Labels).To(HaveKeyWithValue(mandatoryLabel, "enforced"), pvc.Name)
					g.Expect(pvc.Annotations).To(HaveKeyWithValue(ruleKey, "pvcs"), pvc.Name)
					g.Expect(pvc.Labels).ToNot(HaveKey(ruleKey), pvc.Name)
				}
			}, 120).Should(Succeed())
		})

		It("puts the Prometheus discovery annotations on the instance Pods only", func() {
			Eventually(func(g Gomega) {
				podList, err := clusterutils.ListPods(env.Ctx, env.Client, namespace, clusterName)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(podList.Items).To(HaveLen(2))
				for _, pod := range podList.Items {
					g.Expect(pod.Annotations).To(HaveKeyWithValue(scrapeKey, "true"), pod.Name)
					g.Expect(pod.Annotations).To(HaveKeyWithValue("prometheus.io/port", "9187"), pod.Name)
					g.Expect(pod.Annotations).To(HaveKeyWithValue("prometheus.io/path", "/metrics"), pod.Name)
					g.Expect(pod.Annotations).To(HaveKeyWithValue("prometheus.io/scheme", "http"), pod.Name)
				}
			}, 120).Should(Succeed())

			// the Pods of the Jobs serve no metrics
			for _, pod := range []corev1.Pod{initdbJobPod, joinJobPod} {
				Expect(pod.Annotations).ToNot(HaveKey(scrapeKey), pod.Name)
			}

			// "Services never get them, so that no agent scrapes a pod again
			// through a service"
			for _, suffix := range []string{"-rw", "-ro", "-r"} {
				service := &corev1.Service{}
				Expect(env.Client.Get(env.Ctx,
					ctrlclient.ObjectKey{Namespace: namespace, Name: clusterName + suffix}, service)).To(Succeed())
				Expect(service.Annotations).ToNot(HaveKey(scrapeKey), service.Name)
			}
		})
	})
