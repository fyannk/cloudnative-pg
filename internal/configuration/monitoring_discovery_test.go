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

package configuration

import (
	"github.com/cloudnative-pg/cloudnative-pg/pkg/monitoringdiscovery"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("MONITORING_DISCOVERY_ANNOTATIONS", func() {
	It("is empty by default", func() {
		config := NewConfiguration()
		Expect(config.ParseMonitoringDiscoveryAnnotations()).To(Succeed())
		Expect(config.MonitoringDiscoveryFamilies()).To(BeEmpty())
	})

	It("gives nothing before it is parsed", func() {
		config := NewConfiguration()
		config.MonitoringDiscoveryAnnotations = []string{"prometheus"}
		Expect(config.MonitoringDiscoveryFamilies()).To(BeEmpty())
	})

	It("reads the families from the operator ConfigMap, trimming them", func() {
		config := NewConfiguration()
		config.ReadConfigMap(map[string]string{"MONITORING_DISCOVERY_ANNOTATIONS": " opentelemetry , prometheus "})
		Expect(config.ParseMonitoringDiscoveryAnnotations()).To(Succeed())
		Expect(config.MonitoringDiscoveryFamilies()).To(Equal(
			[]monitoringdiscovery.Family{monitoringdiscovery.Prometheus, monitoringdiscovery.OpenTelemetry}))
	})

	It("refuses an unknown family, naming the setting", func() {
		config := NewConfiguration()
		config.ReadConfigMap(map[string]string{"MONITORING_DISCOVERY_ANNOTATIONS": "prometheus,datadog"})
		err := config.ParseMonitoringDiscoveryAnnotations()
		Expect(err).To(MatchError(ContainSubstring("MONITORING_DISCOVERY_ANNOTATIONS")))
		Expect(err).To(MatchError(ContainSubstring(`"datadog"`)))
		Expect(config.MonitoringDiscoveryFamilies()).To(BeEmpty())
	})

	It("keeps the previous families when a new value is refused", func() {
		config := NewConfiguration()
		config.MonitoringDiscoveryAnnotations = []string{"prometheus"}
		Expect(config.ParseMonitoringDiscoveryAnnotations()).To(Succeed())
		config.MonitoringDiscoveryAnnotations = []string{"otel"}
		Expect(config.ParseMonitoringDiscoveryAnnotations()).ToNot(Succeed())
		Expect(config.MonitoringDiscoveryFamilies()).To(Equal([]monitoringdiscovery.Family{monitoringdiscovery.Prometheus}))
	})

	It("returns a copy, which the caller may change", func() {
		config := NewConfiguration()
		config.MonitoringDiscoveryAnnotations = []string{"prometheus", "opentelemetry"}
		Expect(config.ParseMonitoringDiscoveryAnnotations()).To(Succeed())
		families := config.MonitoringDiscoveryFamilies()
		families[0] = "changed"
		Expect(config.MonitoringDiscoveryFamilies()[0]).To(Equal(monitoringdiscovery.Prometheus))
	})
})
