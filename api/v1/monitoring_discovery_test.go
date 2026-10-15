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
	"github.com/cloudnative-pg/cloudnative-pg/pkg/monitoringdiscovery"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The monitoring discovery annotations describe what the instance and Pooler
// Pods serve: they go on those Pods only, over every other source, and follow
// the metrics TLS of each Cluster and Pooler
var _ = Describe("The monitoring discovery annotations", func() {
	var cluster *Cluster

	families := func(values ...string) {
		configuration.Current.MonitoringDiscoveryAnnotations = values
		Expect(configuration.Current.ParseMonitoringDiscoveryAnnotations()).To(Succeed())
	}
	withoutVersion := func(annotations map[string]string) map[string]string {
		delete(annotations, utils.OperatorVersionAnnotationName)
		return annotations
	}

	BeforeEach(func() {
		previous := configuration.Current
		configuration.Current = configuration.NewConfiguration()
		DeferCleanup(func() { configuration.Current = previous })

		cluster = &Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: "payments"},
			Spec: ClusterSpec{
				InheritedMetadata: &EmbeddedObjectMetadata{Annotations: map[string]string{"note": "inherited"}},
			},
		}
	})

	It("are none by default, and change nothing", func() {
		Expect(cluster.GetMonitoringDiscoveryAnnotations()).To(BeNil())
		for _, target := range ClusterMetadataTargets {
			Expect(withoutVersion(cluster.GetInheritedAnnotationsFor(target))).To(
				Equal(map[string]string{"note": "inherited"}), string(target))
		}
		Expect((&Pooler{}).GetMonitoringDiscoveryAnnotations()).To(BeNil())
	})

	It("go on the instance Pods only", func() {
		families("prometheus")
		for _, target := range ClusterMetadataTargets {
			annotations := cluster.GetInheritedAnnotationsFor(target)
			if target == metadatarules.PgPods {
				Expect(annotations).To(HaveKeyWithValue("prometheus.io/port", "9187"))
			} else {
				Expect(annotations).NotTo(HaveKey("prometheus.io/port"), string(target))
			}
			Expect(annotations).To(HaveKeyWithValue("note", "inherited"), string(target))
		}
	})

	It("describe the metrics port, and the logs of the postgres container", func() {
		families("prometheus", "opentelemetry")
		Expect(cluster.GetMonitoringDiscoveryAnnotations()).To(Equal(monitoringdiscovery.Annotations(
			[]monitoringdiscovery.Family{monitoringdiscovery.Prometheus, monitoringdiscovery.OpenTelemetry},
			monitoringdiscovery.Endpoint{
				MetricsPort: 9187, JobName: "cloudnative-pg", LogsContainer: "postgres",
			})))
	})

	It("follow the metrics TLS of the Cluster", func() {
		families("prometheus")
		Expect(cluster.GetInheritedAnnotationsFor(metadatarules.PgPods)).To(
			HaveKeyWithValue("prometheus.io/scheme", "http"))
		cluster.Spec.Monitoring = &MonitoringConfiguration{TLSConfig: &ClusterMonitoringTLSConfiguration{Enabled: true}}
		Expect(cluster.GetInheritedAnnotationsFor(metadatarules.PgPods)).To(
			HaveKeyWithValue("prometheus.io/scheme", "https"))
		cluster.Spec.Monitoring.TLSConfig.Enabled = false
		Expect(cluster.GetInheritedAnnotationsFor(metadatarules.PgPods)).To(
			HaveKeyWithValue("prometheus.io/scheme", "http"))
	})

	It("win over the Cluster's metadata and the default and mandatory annotations", func() {
		configuration.Current.DefaultMetadata = `[{"targets": ["pgPods"], "annotations": {"prometheus.io/path": "/default"}}]`
		configuration.Current.MandatoryMetadata = `[{"targets": ["all"],
			"annotations": {"prometheus.io/port": "1234", "example.com/owner": "x"}}]`
		Expect(configuration.Current.ParseDefaultAndMandatoryMetadata()).To(Succeed())
		families("prometheus")
		cluster.Spec.InheritedMetadata.Annotations["prometheus.io/scrape"] = "false"
		cluster.Spec.MetadataPropagation = []MetadataPropagationRule{
			rule(nil, map[string]string{"prometheus.io/scheme": "ftp"}, "pgPods"),
		}

		pods := cluster.GetInheritedAnnotationsFor(metadatarules.PgPods)
		Expect(pods).To(HaveKeyWithValue("prometheus.io/scrape", "true"))
		Expect(pods).To(HaveKeyWithValue("prometheus.io/port", "9187"))
		Expect(pods).To(HaveKeyWithValue("prometheus.io/path", "/metrics"))
		Expect(pods).To(HaveKeyWithValue("prometheus.io/scheme", "http"))
		Expect(pods).To(HaveKeyWithValue("example.com/owner", "x"))

		// Elsewhere, the other sources are untouched
		services := cluster.GetInheritedAnnotationsFor(metadatarules.Services)
		Expect(services).To(HaveKeyWithValue("prometheus.io/port", "1234"))
		Expect(services).To(HaveKeyWithValue("prometheus.io/scrape", "false"))
	})

	It("are kept apart from what the Cluster asks for", func() {
		families("prometheus")
		Expect(cluster.GetClusterAnnotationsFor(metadatarules.PgPods)).NotTo(
			HaveKey("prometheus.io/port"))
	})

	It("go on the Pods with SetInheritedDataFor, keeping their other annotations", func() {
		families("opentelemetry")
		meta := metav1.ObjectMeta{Annotations: map[string]string{"third-party": "kept"}}
		cluster.SetInheritedDataFor(&meta, metadatarules.PgPods)
		Expect(meta.Annotations).To(HaveKeyWithValue("io.opentelemetry.discovery.metrics.9187/enabled", "true"))
		Expect(meta.Annotations).To(HaveKeyWithValue("third-party", "kept"))
	})

	Context("of a Pooler", func() {
		It("describe the PgBouncer metrics port, and the logs of the pgbouncer container", func() {
			families("opentelemetry")
			pooler := &Pooler{}
			Expect(pooler.GetMonitoringDiscoveryAnnotations()).To(Equal(monitoringdiscovery.Annotations(
				[]monitoringdiscovery.Family{monitoringdiscovery.OpenTelemetry},
				monitoringdiscovery.Endpoint{
					MetricsPort: 9127, JobName: "cloudnative-pg-pooler", LogsContainer: "pgbouncer",
				})))
		})

		It("follow the metrics TLS of the Pooler, not that of its Cluster", func() {
			families("prometheus")
			pooler := &Pooler{Spec: PoolerSpec{Monitoring: &PoolerMonitoringConfiguration{
				TLSConfig: &PoolerMonitoringTLSConfiguration{Enabled: true},
			}}}
			Expect(pooler.GetMonitoringDiscoveryAnnotations()).To(HaveKeyWithValue("prometheus.io/scheme", "https"))
			pooler.Spec.Monitoring.TLSConfig = nil
			Expect(pooler.GetMonitoringDiscoveryAnnotations()).To(HaveKeyWithValue("prometheus.io/scheme", "http"))
		})
	})
})
