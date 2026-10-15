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
	"fmt"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/internal/configuration"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/metadatarules"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The monitoring discovery annotations win over the annotations a Cluster or
// a Pooler sets on its Pods: the webhook tells which ones, with the value
// that really applies
var _ = Describe("Warnings about the monitoring discovery annotations", func() {
	families := func(values ...string) {
		configuration.Current.MonitoringDiscoveryAnnotations = values
		Expect(configuration.Current.ParseMonitoringDiscoveryAnnotations()).To(Succeed())
	}
	mandatory := func(yaml string) {
		configuration.Current.MandatoryMetadata = yaml
		Expect(configuration.Current.ParseDefaultAndMandatoryMetadata()).To(Succeed())
	}
	clusterWarnings := func(cluster *apiv1.Cluster) []string {
		return getReplacedByDiscoveryWarnings(metadatarules.ClusterResource, metadatarules.PgPods,
			cluster.GetClusterAnnotationsFor(metadatarules.PgPods), cluster.GetMonitoringDiscoveryAnnotations())
	}
	poolerWarnings := func(pooler *apiv1.Pooler) []string {
		return getReplacedByDiscoveryWarnings(metadatarules.PoolerResource, metadatarules.PoolerPods,
			pooler.GetPoolerAnnotationsFor(metadatarules.PoolerPods), pooler.GetMonitoringDiscoveryAnnotations())
	}
	replaced := func(resource, key, value, setting, newValue, target string) string {
		return fmt.Sprintf("annotation %q: %q set on this %s is replaced with %q by %s on %s",
			key, value, resource, newValue, setting, target)
	}

	BeforeEach(func() {
		previous := configuration.Current
		configuration.Current = configuration.NewConfiguration()
		DeferCleanup(func() { configuration.Current = previous })
	})

	clusterSetting := func(annotations map[string]string) *apiv1.Cluster {
		return &apiv1.Cluster{Spec: apiv1.ClusterSpec{MetadataPropagation: []apiv1.MetadataPropagationRule{{
			Targets:     []apiv1.MetadataPropagationTarget{"pgPods"},
			Annotations: annotations,
		}}}}
	}

	Context("on a Cluster", func() {
		It("says nothing without a family, whatever the Cluster sets", func() {
			cluster := clusterSetting(map[string]string{"prometheus.io/port": "8080"})
			Expect(clusterWarnings(cluster)).To(BeEmpty())
		})

		It("says nothing when the Cluster sets the same value, or other keys", func() {
			families("prometheus")
			cluster := clusterSetting(map[string]string{"prometheus.io/port": "9187", "example.com/x": "y"})
			Expect(clusterWarnings(cluster)).To(BeEmpty())
		})

		It("names each replaced annotation with the value that applies", func() {
			families("prometheus", "opentelemetry")
			cluster := clusterSetting(map[string]string{
				"prometheus.io/port": "8080",
				"io.opentelemetry.discovery.logs.postgres/enabled": "false",
			})
			Expect(clusterWarnings(cluster)).To(Equal([]string{
				replaced("Cluster", "io.opentelemetry.discovery.logs.postgres/enabled", "false",
					"MONITORING_DISCOVERY_ANNOTATIONS", "true", "pgPods"),
				replaced("Cluster", "prometheus.io/port", "8080", "MONITORING_DISCOVERY_ANNOTATIONS", "9187", "pgPods"),
			}))
		})

		It("sees the annotations of inheritedMetadata too", func() {
			families("prometheus")
			cluster := &apiv1.Cluster{Spec: apiv1.ClusterSpec{InheritedMetadata: &apiv1.EmbeddedObjectMetadata{
				Annotations: map[string]string{"prometheus.io/scrape": "false"},
			}}}
			Expect(clusterWarnings(cluster)).To(Equal([]string{
				replaced("Cluster", "prometheus.io/scrape", "false", "MONITORING_DISCOVERY_ANNOTATIONS", "true", "pgPods"),
			}))
		})

		It("follows the metrics TLS of the Cluster", func() {
			families("prometheus")
			cluster := clusterSetting(map[string]string{"prometheus.io/scheme": "http"})
			Expect(clusterWarnings(cluster)).To(BeEmpty())
			cluster.Spec.Monitoring = &apiv1.MonitoringConfiguration{
				TLSConfig: &apiv1.ClusterMonitoringTLSConfiguration{Enabled: true},
			}
			Expect(clusterWarnings(cluster)).To(Equal([]string{
				replaced("Cluster", "prometheus.io/scheme", "http", "MONITORING_DISCOVERY_ANNOTATIONS", "https", "pgPods"),
			}))
		})

		It("reports a key replaced by both settings once, with the value that applies", func() {
			families("prometheus")
			mandatory(`[{"targets": ["pgPods"],
				"annotations": {"prometheus.io/port": "1234", "example.com/owner": "platform"}}]`)
			cluster := clusterSetting(map[string]string{"prometheus.io/port": "8080", "example.com/owner": "a"})
			v := &ClusterCustomValidator{}
			warnings := v.getAdmissionWarnings(cluster)
			Expect(warnings).To(ContainElement(
				replaced("Cluster", "prometheus.io/port", "8080", "MONITORING_DISCOVERY_ANNOTATIONS", "9187", "pgPods")))
			Expect(warnings).To(ContainElement(
				replaced("Cluster", "example.com/owner", "a", "MANDATORY_METADATA", "platform", "pgPods")))
			Expect(warnings).NotTo(ContainElement(ContainSubstring(`replaced with "1234"`)))
		})

		It("keeps the mandatory warnings of the other kinds of objects", func() {
			families("prometheus")
			mandatory(`[{"targets": ["services"], "annotations": {"prometheus.io/port": "1234"}}]`)
			cluster := &apiv1.Cluster{Spec: apiv1.ClusterSpec{MetadataPropagation: []apiv1.MetadataPropagationRule{{
				Targets:     []apiv1.MetadataPropagationTarget{"services"},
				Annotations: map[string]string{"prometheus.io/port": "8080"},
			}}}}
			Expect([]string(getMetadataPropagationWarnings(cluster))).To(Equal([]string{
				replaced("Cluster", "prometheus.io/port", "8080", "MANDATORY_METADATA", "1234", "services"),
			}))
		})
	})

	Context("on a Pooler", func() {
		poolerSetting := func(annotations map[string]string) *apiv1.Pooler {
			return &apiv1.Pooler{Spec: apiv1.PoolerSpec{
				Cluster:  apiv1.LocalObjectReference{Name: "payments"},
				Template: &apiv1.PodTemplateSpec{ObjectMeta: apiv1.Metadata{Annotations: annotations}},
			}}
		}

		It("says nothing without a family, or without a template", func() {
			Expect(poolerWarnings(
				poolerSetting(map[string]string{"prometheus.io/port": "8080"}))).To(BeEmpty())
			families("prometheus")
			Expect(poolerWarnings(&apiv1.Pooler{})).To(BeEmpty())
		})

		It("names the replaced annotations of the pod template, with the Pooler's TLS", func() {
			families("prometheus")
			pooler := poolerSetting(map[string]string{"prometheus.io/port": "8080", "prometheus.io/scheme": "http"})
			pooler.Spec.Monitoring = &apiv1.PoolerMonitoringConfiguration{
				TLSConfig: &apiv1.PoolerMonitoringTLSConfiguration{Enabled: true},
			}
			Expect(poolerWarnings(pooler)).To(Equal([]string{
				replaced("Pooler", "prometheus.io/port", "8080", "MONITORING_DISCOVERY_ANNOTATIONS", "9127", "poolerPods"),
				replaced("Pooler", "prometheus.io/scheme", "http", "MONITORING_DISCOVERY_ANNOTATIONS", "https",
					"poolerPods"),
			}))
		})

		It("sees the rules of the Pooler for its Pods, and for every object", func() {
			families("prometheus")
			pooler := poolerSetting(nil)
			pooler.Spec.MetadataPropagation = []apiv1.MetadataPropagationRule{
				{
					Targets:     []apiv1.MetadataPropagationTarget{"poolerPods"},
					Annotations: map[string]string{"prometheus.io/port": "8080"},
				},
				{
					Targets:     []apiv1.MetadataPropagationTarget{"all"},
					Annotations: map[string]string{"prometheus.io/scrape": "false"},
				},
			}
			Expect(poolerWarnings(pooler)).To(Equal([]string{
				replaced("Pooler", "prometheus.io/port", "8080", "MONITORING_DISCOVERY_ANNOTATIONS", "9127", "poolerPods"),
				replaced("Pooler", "prometheus.io/scrape", "false", "MONITORING_DISCOVERY_ANNOTATIONS", "true",
					"poolerPods"),
			}))
		})

		It("reports a key replaced by both settings once, with the value that applies", func() {
			families("prometheus")
			mandatory(`[{"targets": ["poolerPods"], "annotations": {"prometheus.io/port": "1234"}}]`)
			pooler := poolerSetting(map[string]string{"prometheus.io/port": "8080"})
			v := &PoolerCustomValidator{}
			Expect([]string(v.getAdmissionWarnings(pooler))).To(ContainElement(
				replaced("Pooler", "prometheus.io/port", "8080", "MONITORING_DISCOVERY_ANNOTATIONS", "9127", "poolerPods")))
			Expect([]string(v.getAdmissionWarnings(pooler))).NotTo(ContainElement(ContainSubstring(`"1234"`)))
		})
	})
})
