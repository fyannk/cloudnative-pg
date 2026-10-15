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

// Package monitoringdiscovery renders the annotations that tell monitoring
// agents how to collect the metrics and the logs of a Pod: the
// `prometheus.io/*` annotations, and the `io.opentelemetry.discovery.*`
// hints of the OpenTelemetry Collector. The names of the annotations and
// their content are kept here only, to follow the agents if they change.
package monitoringdiscovery

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Family is a family of discovery annotations
type Family string

const (
	// Prometheus is the family of the `prometheus.io/*` annotations
	Prometheus Family = "prometheus"

	// OpenTelemetry is the family of the `io.opentelemetry.discovery.*`
	// hints, read by the receiver_creator of the OpenTelemetry Collector
	OpenTelemetry Family = "opentelemetry"
)

// Families lists the known families, in the order they are rendered
var Families = []Family{Prometheus, OpenTelemetry}

// ParseFamilies parses a list of families, refusing an unknown one. The
// result has no duplicate, and keeps the order of Families
func ParseFamilies(values []string) ([]Family, error) {
	var errs []string
	requested := make(map[Family]bool, len(values))
	for _, value := range values {
		family := Family(strings.TrimSpace(value))
		if family == "" {
			continue
		}
		if !slices.Contains(Families, family) {
			errs = append(errs, fmt.Sprintf("unknown family %q, expected one of %v", family, Families))
			continue
		}
		requested[family] = true
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(errs, ", "))
	}

	var result []Family
	for _, family := range Families {
		if requested[family] {
			result = append(result, family)
		}
	}

	return result, nil
}

// Endpoint is where and how a Pod serves its metrics, and which container
// writes the logs to collect
type Endpoint struct {
	// MetricsPort is the port of the metrics, in the container
	MetricsPort int32

	// MetricsTLS is true when the metrics are served over HTTPS, with a
	// certificate that does not cover the Pod IP
	MetricsTLS bool

	// JobName is the name of the scrape job in the OpenTelemetry hint
	JobName string

	// LogsContainer is the container whose JSON logs are collected
	LogsContainer string
}

const (
	prometheusPrefix    = "prometheus.io/"
	openTelemetryPrefix = "io.opentelemetry.discovery."
	metricsPath         = "/metrics"
)

// Annotations returns the annotations of the given families for a Pod
// serving the given endpoint. It returns nil when no family is given
func Annotations(families []Family, endpoint Endpoint) map[string]string {
	if len(families) == 0 {
		return nil
	}

	result := make(map[string]string)
	for _, family := range families {
		switch family {
		case Prometheus:
			addPrometheusAnnotations(result, endpoint)
		case OpenTelemetry:
			addOpenTelemetryAnnotations(result, endpoint)
		}
	}

	return result
}

func scheme(endpoint Endpoint) string {
	if endpoint.MetricsTLS {
		return "https"
	}
	return "http"
}

func addPrometheusAnnotations(result map[string]string, endpoint Endpoint) {
	result[prometheusPrefix+"scrape"] = "true"
	result[prometheusPrefix+"port"] = strconv.Itoa(int(endpoint.MetricsPort))
	result[prometheusPrefix+"path"] = metricsPath
	result[prometheusPrefix+"scheme"] = scheme(endpoint)
}

// addOpenTelemetryAnnotations adds the hints of the receiver_creator: the
// metrics of the port, scraped by the `prometheus` receiver, and the logs of
// the container, parsed as JSON. The logs operators replace the default
// ones of the receiver_creator, hence the container parser first
func addOpenTelemetryAnnotations(result map[string]string, endpoint Endpoint) {
	metrics := fmt.Sprintf("%smetrics.%d/", openTelemetryPrefix, endpoint.MetricsPort)
	result[metrics+"enabled"] = "true"
	result[metrics+"scraper"] = "prometheus"

	var config strings.Builder
	config.WriteString("config:\n")
	config.WriteString("  scrape_configs:\n")
	fmt.Fprintf(&config, "    - job_name: %s\n", endpoint.JobName)
	fmt.Fprintf(&config, "      metrics_path: %s\n", metricsPath)
	fmt.Fprintf(&config, "      scheme: %s\n", scheme(endpoint))
	if endpoint.MetricsTLS {
		config.WriteString("      tls_config:\n")
		config.WriteString("        insecure_skip_verify: true\n")
	}
	config.WriteString("      static_configs:\n")
	config.WriteString("        - targets: [\"`endpoint`\"]\n")
	result[metrics+"config"] = config.String()

	logs := fmt.Sprintf("%slogs.%s/", openTelemetryPrefix, endpoint.LogsContainer)
	result[logs+"enabled"] = "true"
	result[logs+"config"] = "operators:\n" +
		"  - type: container\n" +
		"    id: container-parser\n" +
		"  - type: json_parser\n"
}
