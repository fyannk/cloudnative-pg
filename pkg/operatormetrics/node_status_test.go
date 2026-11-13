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

package operatormetrics

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/cloudnative-pg/cloudnative-pg/internal/configuration"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("The node state availability metric", func() {
	// scrape returns the policy label and the value of the only series
	scrape := func(conf *configuration.Data) (string, float64) {
		registry := prometheus.NewPedanticRegistry()
		Expect(registry.Register(NewNodeStatusCollector(conf))).To(Succeed())
		families, err := registry.Gather()
		Expect(err).ToNot(HaveOccurred())
		Expect(families).To(HaveLen(1))
		Expect(families[0].GetName()).To(Equal("cnpg_operator_node_status_unavailable"))
		Expect(families[0].GetMetric()).To(HaveLen(1))
		metric := families[0].GetMetric()[0]
		Expect(metric.GetLabel()).To(HaveLen(1))
		Expect(metric.GetLabel()[0].GetName()).To(Equal("policy"))
		return metric.GetLabel()[0].GetValue(), metric.GetGauge().GetValue()
	}

	DescribeTable("reports the resolved source and the policy",
		func(source, policy, expectedPolicy string, expectedValue float64) {
			conf := configuration.NewConfiguration()
			conf.NodeStatusSource = source
			conf.NodeStatusUnavailablePolicy = policy
			gotPolicy, gotValue := scrape(conf)
			Expect(gotPolicy).To(Equal(expectedPolicy))
			Expect(gotValue).To(Equal(expectedValue))
		},
		Entry("none, unset policy", configuration.NodeStatusSourceNone, "",
			configuration.NodeStatusUnavailablePolicyKeepPDB, 1.0),
		Entry("none, disablePrimaryPDB", configuration.NodeStatusSourceNone,
			configuration.NodeStatusUnavailablePolicyDisablePrimaryPDB,
			configuration.NodeStatusUnavailablePolicyDisablePrimaryPDB, 1.0),
		Entry("api", configuration.NodeStatusSourceAPI, configuration.NodeStatusUnavailablePolicyKeepPDB,
			configuration.NodeStatusUnavailablePolicyKeepPDB, 0.0),
		Entry("pods", configuration.NodeStatusSourcePods, configuration.NodeStatusUnavailablePolicyKeepPDB,
			configuration.NodeStatusUnavailablePolicyKeepPDB, 0.0),
	)
})
