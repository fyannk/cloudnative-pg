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

package specs

import (
	"regexp"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/internal/configuration"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The port and the container the discovery annotations name must be those
// the instance Pods really serve and run: a guard against a change of either
// on one side only
var _ = Describe("The monitoring discovery annotations of the instance Pods", func() {
	It("name the metrics port and the container of the instance Pods", func(ctx SpecContext) {
		previous := configuration.Current
		configuration.Current = configuration.NewConfiguration()
		DeferCleanup(func() { configuration.Current = previous })
		configuration.Current.MonitoringDiscoveryAnnotations = []string{"prometheus", "opentelemetry"}
		Expect(configuration.Current.ParseMonitoringDiscoveryAnnotations()).To(Succeed())

		cluster := apiv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: "payments", Namespace: "payments"},
			Spec:       apiv1.ClusterSpec{ImageName: "postgres:18.0"},
		}
		pod, err := NewInstance(ctx, cluster, 1)
		Expect(err).ToNot(HaveOccurred())

		annotations := cluster.GetMonitoringDiscoveryAnnotations()
		port, err := strconv.Atoi(annotations["prometheus.io/port"])
		Expect(err).ToNot(HaveOccurred())

		metricsKey := regexp.MustCompile(`^io\.opentelemetry\.discovery\.metrics\.(\d+)/`)
		logsKey := regexp.MustCompile(`^io\.opentelemetry\.discovery\.logs\.([^/]+)/`)
		var metricsKeys, logsKeys []string
		for key := range annotations {
			if match := metricsKey.FindStringSubmatch(key); match != nil {
				metricsKeys = append(metricsKeys, match[1])
			}
			if match := logsKey.FindStringSubmatch(key); match != nil {
				logsKeys = append(logsKeys, match[1])
			}
		}
		Expect(metricsKeys).To(HaveEach(strconv.Itoa(port)))
		Expect(logsKeys).To(HaveEach(PostgresContainerName))

		var found bool
		for _, container := range pod.Spec.Containers {
			if container.Name != PostgresContainerName {
				continue
			}
			for _, containerPort := range container.Ports {
				if containerPort.Name == "metrics" {
					Expect(int(containerPort.ContainerPort)).To(Equal(port))
					found = true
				}
			}
		}
		Expect(found).To(BeTrue(), "the postgres container exposes the metrics port")
	})
})
