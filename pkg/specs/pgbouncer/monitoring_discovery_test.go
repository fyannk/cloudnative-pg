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

package pgbouncer

import (
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	config "github.com/cloudnative-pg/cloudnative-pg/internal/configuration"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The Pooler Pods get the monitoring discovery annotations through the pod
// template of their Deployment. The hash must include them only when there
// are some: otherwise upgrading the operator would roll out every Pooler
var _ = Describe("The monitoring discovery annotations of the Pooler Pods", func() {
	var (
		pooler  *apiv1.Pooler
		cluster *apiv1.Cluster
	)

	families := func(values ...string) {
		config.Current.MonitoringDiscoveryAnnotations = values
		Expect(config.Current.ParseMonitoringDiscoveryAnnotations()).To(Succeed())
	}
	deployment := func() *appsv1.Deployment {
		result, err := Deployment(pooler, cluster)
		Expect(err).ToNot(HaveOccurred())
		return result
	}
	hashOf := func(deployment *appsv1.Deployment) string {
		return deployment.Annotations[utils.PoolerSpecHashAnnotationName]
	}

	BeforeEach(func() {
		previous := config.Current
		config.Current = config.NewConfiguration()
		DeferCleanup(func() { config.Current = previous })

		pooler = &apiv1.Pooler{
			TypeMeta:   metav1.TypeMeta{Kind: apiv1.PoolerKind, APIVersion: apiv1.SchemeGroupVersion.String()},
			ObjectMeta: metav1.ObjectMeta{Name: "pooler-payments", Namespace: "payments"},
			Spec: apiv1.PoolerSpec{
				Cluster:   apiv1.LocalObjectReference{Name: "payments"},
				Type:      apiv1.PoolerTypeRW,
				Instances: ptr.To(int32(1)),
				PgBouncer: &apiv1.PgBouncerSpec{PoolMode: apiv1.PgBouncerPoolModeSession},
			},
			Status: apiv1.PoolerStatus{Image: config.Current.PgbouncerImageName, Secrets: &apiv1.PoolerSecrets{}},
		}
		cluster = &apiv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "payments", Namespace: "payments"}}
	})

	It("are none by default: the template and the hash are those of stock", func() {
		result := deployment()
		Expect(result.Spec.Template.Annotations).NotTo(HaveKey("prometheus.io/scrape"))
		Expect(hashOf(result)).To(Equal(mustComputeTemplateHash(pooler)))
	})

	It("go on the Pods, not on the Deployment nor the Service", func() {
		families("prometheus", "opentelemetry")
		result := deployment()
		Expect(result.Spec.Template.Annotations).To(HaveKeyWithValue("prometheus.io/port", "9127"))
		Expect(result.Spec.Template.Annotations).To(
			HaveKeyWithValue("io.opentelemetry.discovery.logs.pgbouncer/enabled", "true"))
		Expect(result.Annotations).NotTo(HaveKey("prometheus.io/port"))

		service, err := Service(pooler, cluster)
		Expect(err).ToNot(HaveOccurred())
		Expect(service.Annotations).NotTo(HaveKey("prometheus.io/port"))
		Expect(service.Annotations).NotTo(HaveKey("io.opentelemetry.discovery.metrics.9127/enabled"))
	})

	It("name the metrics port and the container of the Pooler Pods", func() {
		families("prometheus", "opentelemetry")
		result := deployment()
		port, err := strconv.Atoi(result.Spec.Template.Annotations["prometheus.io/port"])
		Expect(err).ToNot(HaveOccurred())

		var found bool
		for _, container := range result.Spec.Template.Spec.Containers {
			if container.Name != "pgbouncer" {
				continue
			}
			for _, containerPort := range container.Ports {
				if containerPort.Name == "metrics" {
					Expect(int(containerPort.ContainerPort)).To(Equal(port))
					found = true
				}
			}
		}
		Expect(found).To(BeTrue(), "the pgbouncer container exposes the metrics port")
		Expect(result.Spec.Template.Annotations).To(
			HaveKey("io.opentelemetry.discovery.metrics." + strconv.Itoa(port) + "/enabled"))
	})

	It("win over the Pooler's template and the mandatory annotations", func() {
		config.Current.MandatoryMetadata = `[{"targets": ["poolerPods"], "annotations": {"prometheus.io/port": "1234"}}]`
		Expect(config.Current.ParseDefaultAndMandatoryMetadata()).To(Succeed())
		families("prometheus")
		pooler.Spec.Template = &apiv1.PodTemplateSpec{ObjectMeta: apiv1.Metadata{
			Annotations: map[string]string{"prometheus.io/scrape": "false", "example.com/mesh": "on"},
		}}
		annotations := deployment().Spec.Template.Annotations
		Expect(annotations).To(HaveKeyWithValue("prometheus.io/scrape", "true"))
		Expect(annotations).To(HaveKeyWithValue("prometheus.io/port", "9127"))
		Expect(annotations).To(HaveKeyWithValue("example.com/mesh", "on"))
	})

	It("change the hash, the same at every reconciliation", func() {
		families("prometheus")
		first := hashOf(deployment())
		Expect(first).NotTo(Equal(mustComputeTemplateHash(pooler)))
		for range 20 {
			Expect(hashOf(deployment())).To(Equal(first))
		}
	})

	DescribeTable("change the hash again when the annotations change",
		func(change func()) {
			families("prometheus")
			before := hashOf(deployment())
			change()
			Expect(hashOf(deployment())).NotTo(Equal(before))
		},
		Entry("another family", func() { families("prometheus", "opentelemetry") }),
		Entry("no family any more", func() { families() }),
		Entry("the metrics TLS of the Pooler", func() {
			pooler.Spec.Monitoring = &apiv1.PoolerMonitoringConfiguration{
				TLSConfig: &apiv1.PoolerMonitoringTLSConfiguration{Enabled: true},
			}
		}),
	)

	It("follow the metrics TLS of the Pooler", func() {
		families("prometheus")
		pooler.Spec.Monitoring = &apiv1.PoolerMonitoringConfiguration{
			TLSConfig: &apiv1.PoolerMonitoringTLSConfiguration{Enabled: true},
		}
		Expect(deployment().Spec.Template.Annotations).To(HaveKeyWithValue("prometheus.io/scheme", "https"))
	})

	It("give back the stock hash when turned off", func() {
		families("prometheus")
		deployment()
		families()
		Expect(hashOf(deployment())).To(Equal(mustComputeTemplateHash(pooler)))
	})
})
