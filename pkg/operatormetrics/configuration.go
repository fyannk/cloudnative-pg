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
	"fmt"
	"reflect"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	"github.com/cloudnative-pg/cloudnative-pg/internal/configuration"
)

// RedactedValue replaces the value of a setting read from the operator Secret
const RedactedValue = "<redacted>"

var configInfoDesc = prometheus.NewDesc(
	"cnpg_operator_config_info",
	"Effective value of each operator setting, defaults included; "+
		"the settings read from the operator Secret are redacted",
	[]string{"name", "value"},
	nil,
)

// ConfigCollector reports the effective operator configuration. The values
// are fixed once the operator has started, so they are captured when the
// collector is created and a scrape costs nothing.
type ConfigCollector struct {
	metrics []prometheus.Metric
}

// NewConfigCollector captures one series per setting of the configuration,
// named as its environment variable. The settings named in fromSecret were
// read from the operator Secret: their value is redacted.
func NewConfigCollector(conf *configuration.Data, fromSecret []string) *ConfigCollector {
	redacted := make(map[string]bool, len(fromSecret))
	for _, name := range fromSecret {
		redacted[name] = true
	}

	collector := &ConfigCollector{}
	value := reflect.ValueOf(conf).Elem()
	for i := range value.NumField() {
		name := value.Type().Field(i).Tag.Get("env")
		if name == "" {
			continue
		}
		settingValue := formatSetting(value.Field(i))
		if redacted[name] {
			settingValue = RedactedValue
		}
		collector.metrics = append(collector.metrics,
			prometheus.MustNewConstMetric(configInfoDesc, prometheus.GaugeValue, 1, name, settingValue))
	}
	return collector
}

// RegisterConfiguration registers a configuration collector on the metrics
// registry of controller-runtime
func RegisterConfiguration(conf *configuration.Data, fromSecret []string) error {
	return metrics.Registry.Register(NewConfigCollector(conf, fromSecret))
}

// formatSetting renders a setting as the operator reads it: lists
// comma-separated, an unset optional value as an empty string
func formatSetting(value reflect.Value) string {
	switch value.Kind() {
	case reflect.Pointer:
		if value.IsNil() {
			return ""
		}
		return formatSetting(value.Elem())
	case reflect.Slice:
		items := make([]string, value.Len())
		for i := range value.Len() {
			items[i] = fmt.Sprint(value.Index(i).Interface())
		}
		return strings.Join(items, ",")
	default:
		return fmt.Sprint(value.Interface())
	}
}

// Describe implements prometheus.Collector
func (c *ConfigCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- configInfoDesc
}

// Collect implements prometheus.Collector
func (c *ConfigCollector) Collect(ch chan<- prometheus.Metric) {
	for _, metric := range c.metrics {
		ch <- metric
	}
}
