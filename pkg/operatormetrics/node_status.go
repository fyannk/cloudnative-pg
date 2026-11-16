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

// Package operatormetrics exposes the metrics of the operator itself
package operatormetrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/cloudnative-pg/cloudnative-pg/internal/configuration"
)

var nodeStatusUnavailableDesc = prometheus.NewDesc(
	"cnpg_operator_node_status_unavailable",
	"1 when the operator reads no node state (NODE_STATUS_SOURCE resolved to none at startup), "+
		"0 otherwise; policy is the NODE_STATUS_UNAVAILABLE_POLICY applied in that case",
	[]string{"policy"},
	nil,
)

// NodeStatusCollector reports whether the operator runs in the degraded mode
// where it reads no node state. The source of node state is resolved once at
// startup, so the value is captured when the collector is created.
type NodeStatusCollector struct {
	metric prometheus.Metric
}

// NewNodeStatusCollector captures the node state availability of a
// configuration whose NodeStatusSource is already resolved
func NewNodeStatusCollector(conf *configuration.Data) *NodeStatusCollector {
	value := 0.0
	if conf.ReadsNoNodeStatus() {
		value = 1
	}
	return &NodeStatusCollector{
		metric: prometheus.MustNewConstMetric(nodeStatusUnavailableDesc, prometheus.GaugeValue, value,
			conf.GetNodeStatusUnavailablePolicy()),
	}
}

// RegisterNodeStatus registers a node state availability collector on the
// metrics registry of controller-runtime
func RegisterNodeStatus(conf *configuration.Data) error {
	return metrics.Registry.Register(NewNodeStatusCollector(conf))
}

// Describe implements prometheus.Collector
func (c *NodeStatusCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- nodeStatusUnavailableDesc
}

// Collect implements prometheus.Collector
func (c *NodeStatusCollector) Collect(ch chan<- prometheus.Metric) {
	ch <- c.metric
}
