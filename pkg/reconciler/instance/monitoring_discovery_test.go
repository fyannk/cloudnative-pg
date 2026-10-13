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

package instance

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/internal/configuration"
	"github.com/cloudnative-pg/cloudnative-pg/internal/scheme"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The monitoring discovery annotations of an instance Pod are updated in
// place, like the rest of its metadata: when the setting changes, and when
// the metrics TLS of the Cluster does
var _ = Describe("monitoring discovery annotations of the instance Pods, reconciled in place", func() {
	var (
		cluster *apiv1.Cluster
		cli     ctrlclient.Client
	)

	families := func(values ...string) {
		configuration.Current.MonitoringDiscoveryAnnotations = values
		Expect(configuration.Current.ParseMonitoringDiscoveryAnnotations()).To(Succeed())
	}
	reconcileAndGet := func() corev1.Pod {
		var pods corev1.PodList
		Expect(cli.List(context.Background(), &pods)).To(Succeed())
		Expect(ReconcileMetadata(context.Background(), cli, cluster, pods.Items)).To(Succeed())
		var pod corev1.Pod
		Expect(cli.Get(context.Background(), ctrlclient.ObjectKey{Name: "payments-1"}, &pod)).To(Succeed())
		return pod
	}

	BeforeEach(func() {
		previous := configuration.Current
		configuration.Current = configuration.NewConfiguration()
		DeferCleanup(func() { configuration.Current = previous })

		cluster = &apiv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: "payments"},
			Status:     apiv1.ClusterStatus{CurrentPrimary: "payments-1", TargetPrimary: "payments-1"},
		}
		cli = fake.NewClientBuilder().
			WithScheme(scheme.BuildWithAllKnownScheme()).
			WithObjects(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{
				Name:        "payments-1",
				Annotations: map[string]string{"third-party": "kept"},
			}}).
			Build()
	})

	It("adds nothing without a family", func() {
		pod := reconcileAndGet()
		Expect(pod.Annotations).NotTo(HaveKey("prometheus.io/scrape"))
		Expect(pod.Annotations).To(HaveKeyWithValue("third-party", "kept"))
	})

	It("adds them to an existing Pod, then leaves it alone", func() {
		families("prometheus", "opentelemetry")
		first := reconcileAndGet()
		Expect(first.Annotations).To(HaveKeyWithValue("prometheus.io/port", "9187"))
		Expect(first.Annotations).To(HaveKeyWithValue("io.opentelemetry.discovery.logs.postgres/enabled", "true"))
		Expect(first.Annotations).To(HaveKeyWithValue("third-party", "kept"))

		second := reconcileAndGet()
		Expect(second.ResourceVersion).To(Equal(first.ResourceVersion))
	})

	It("follows the metrics TLS of the Cluster", func() {
		families("prometheus", "opentelemetry")
		Expect(reconcileAndGet().Annotations).To(HaveKeyWithValue("prometheus.io/scheme", "http"))

		cluster.Spec.Monitoring = &apiv1.MonitoringConfiguration{
			TLSConfig: &apiv1.ClusterMonitoringTLSConfiguration{Enabled: true},
		}
		pod := reconcileAndGet()
		Expect(pod.Annotations).To(HaveKeyWithValue("prometheus.io/scheme", "https"))
		Expect(pod.Annotations["io.opentelemetry.discovery.metrics.9187/config"]).To(
			ContainSubstring("insecure_skip_verify: true"))

		cluster.Spec.Monitoring.TLSConfig.Enabled = false
		pod = reconcileAndGet()
		Expect(pod.Annotations).To(HaveKeyWithValue("prometheus.io/scheme", "http"))
		Expect(pod.Annotations["io.opentelemetry.discovery.metrics.9187/config"]).NotTo(
			ContainSubstring("tls_config"))
	})

	It("adds a family set later", func() {
		families("prometheus")
		Expect(reconcileAndGet().Annotations).NotTo(HaveKey("io.opentelemetry.discovery.metrics.9187/enabled"))
		families("prometheus", "opentelemetry")
		Expect(reconcileAndGet().Annotations).To(HaveKey("io.opentelemetry.discovery.metrics.9187/enabled"))
	})

	It("does not remove the annotations of a family no longer set, as for inheritedMetadata", func() {
		families("prometheus")
		reconcileAndGet()
		families()
		Expect(reconcileAndGet().Annotations).To(HaveKeyWithValue("prometheus.io/scrape", "true"))
	})
})
