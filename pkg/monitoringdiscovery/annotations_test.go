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

package monitoringdiscovery

import (
	"strings"

	"sigs.k8s.io/yaml"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Parsing the families", func() {
	DescribeTable("accepts the known families, without duplicates, in a fixed order",
		func(values []string, expected []Family) {
			families, err := ParseFamilies(values)
			Expect(err).ToNot(HaveOccurred())
			Expect(families).To(Equal(expected))
		},
		Entry("nothing", nil, nil),
		Entry("an empty list", []string{}, nil),
		Entry("empty values", []string{"", " "}, nil),
		Entry("prometheus", []string{"prometheus"}, []Family{Prometheus}),
		Entry("opentelemetry", []string{"opentelemetry"}, []Family{OpenTelemetry}),
		Entry("both", []string{"prometheus", "opentelemetry"}, []Family{Prometheus, OpenTelemetry}),
		Entry("both, in the other order", []string{"opentelemetry", "prometheus"},
			[]Family{Prometheus, OpenTelemetry}),
		Entry("twice the same", []string{"opentelemetry", "opentelemetry"}, []Family{OpenTelemetry}),
		Entry("spaces around a value", []string{" prometheus "}, []Family{Prometheus}),
	)

	DescribeTable("refuses an unknown family, naming it and the known ones",
		func(values []string, unknown string) {
			_, err := ParseFamilies(values)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`unknown family "` + unknown + `"`))
			Expect(err.Error()).To(ContainSubstring("[prometheus opentelemetry]"))
		},
		Entry("another agent", []string{"datadog"}, "datadog"),
		Entry("the wrong case", []string{"Prometheus"}, "Prometheus"),
		Entry("the collector name", []string{"otel"}, "otel"),
		Entry("next to a valid one", []string{"prometheus", "otlp"}, "otlp"),
	)

	It("names every unknown family at once", func() {
		_, err := ParseFamilies([]string{"datadog", "prometheus", "otel"})
		Expect(err).To(MatchError(ContainSubstring(`"datadog"`)))
		Expect(err).To(MatchError(ContainSubstring(`"otel"`)))
	})
})

var _ = Describe("The annotations of a Pod", func() {
	instance := Endpoint{MetricsPort: 9187, JobName: "cloudnative-pg", LogsContainer: "postgres"}
	instanceTLS := Endpoint{MetricsPort: 9187, MetricsTLS: true, JobName: "cloudnative-pg", LogsContainer: "postgres"}
	pooler := Endpoint{MetricsPort: 9127, JobName: "cloudnative-pg-pooler", LogsContainer: "pgbouncer"}

	It("are none without a family", func() {
		Expect(Annotations(nil, instance)).To(BeNil())
		Expect(Annotations([]Family{}, instanceTLS)).To(BeNil())
	})

	It("ignore an unknown family", func() {
		Expect(Annotations([]Family{"datadog"}, instance)).To(BeEmpty())
	})

	Context("prometheus", func() {
		It("names the port, the path and the scheme", func() {
			Expect(Annotations([]Family{Prometheus}, instance)).To(Equal(map[string]string{
				"prometheus.io/scrape": "true",
				"prometheus.io/port":   "9187",
				"prometheus.io/path":   "/metrics",
				"prometheus.io/scheme": "http",
			}))
		})

		It("switches to https with TLS", func() {
			Expect(Annotations([]Family{Prometheus}, instanceTLS)).To(HaveKeyWithValue("prometheus.io/scheme", "https"))
		})

		It("follows the port of the endpoint", func() {
			Expect(Annotations([]Family{Prometheus}, pooler)).To(HaveKeyWithValue("prometheus.io/port", "9127"))
		})
	})

	Context("opentelemetry", func() {
		// The structure the receiver_creator gives to the prometheus
		// receiver: its own configuration, under `config`
		type scrapeConfig struct {
			JobName     string `json:"job_name"`
			MetricsPath string `json:"metrics_path"`
			Scheme      string `json:"scheme"`
			TLSConfig   *struct {
				InsecureSkipVerify bool `json:"insecure_skip_verify"`
			} `json:"tls_config"`
			StaticConfigs []struct {
				Targets []string `json:"targets"`
			} `json:"static_configs"`
		}
		type metricsConfig struct {
			Config struct {
				ScrapeConfigs []scrapeConfig `json:"scrape_configs"`
			} `json:"config"`
		}
		type logsConfig struct {
			Operators []map[string]string `json:"operators"`
		}
		parseMetrics := func(annotations map[string]string, port string) scrapeConfig {
			var config metricsConfig
			Expect(yaml.UnmarshalStrict(
				[]byte(annotations["io.opentelemetry.discovery.metrics."+port+"/config"]), &config)).To(Succeed())
			Expect(config.Config.ScrapeConfigs).To(HaveLen(1))
			return config.Config.ScrapeConfigs[0]
		}

		It("enables the metrics of the port, scraped by the prometheus receiver", func() {
			annotations := Annotations([]Family{OpenTelemetry}, instance)
			Expect(annotations).To(HaveKeyWithValue("io.opentelemetry.discovery.metrics.9187/enabled", "true"))
			Expect(annotations).To(HaveKeyWithValue("io.opentelemetry.discovery.metrics.9187/scraper", "prometheus"))

			scrape := parseMetrics(annotations, "9187")
			Expect(scrape.JobName).To(Equal("cloudnative-pg"))
			Expect(scrape.MetricsPath).To(Equal("/metrics"))
			Expect(scrape.Scheme).To(Equal("http"))
			Expect(scrape.TLSConfig).To(BeNil())
			Expect(scrape.StaticConfigs).To(HaveLen(1))
			// The receiver_creator replaces `endpoint` with the Pod IP and
			// port: each Pod scrapes itself only
			Expect(scrape.StaticConfigs[0].Targets).To(Equal([]string{"`endpoint`"}))
		})

		It("scrapes over https, without verifying the certificate, with TLS", func() {
			scrape := parseMetrics(Annotations([]Family{OpenTelemetry}, instanceTLS), "9187")
			Expect(scrape.Scheme).To(Equal("https"))
			Expect(scrape.TLSConfig).ToNot(BeNil())
			Expect(scrape.TLSConfig.InsecureSkipVerify).To(BeTrue())
		})

		It("enables the logs of the container, parsed as JSON after the container parser", func() {
			annotations := Annotations([]Family{OpenTelemetry}, instance)
			Expect(annotations).To(HaveKeyWithValue("io.opentelemetry.discovery.logs.postgres/enabled", "true"))

			var config logsConfig
			Expect(yaml.UnmarshalStrict(
				[]byte(annotations["io.opentelemetry.discovery.logs.postgres/config"]), &config)).To(Succeed())
			// The operators replace those of the receiver_creator, which
			// parse the container runtime format: it must come first
			Expect(config.Operators).To(Equal([]map[string]string{
				{"type": "container", "id": "container-parser"},
				{"type": "json_parser"},
			}))
		})

		It("follows the port, the job and the container of the Pooler", func() {
			annotations := Annotations([]Family{OpenTelemetry}, pooler)
			Expect(annotations).To(HaveKeyWithValue("io.opentelemetry.discovery.metrics.9127/enabled", "true"))
			Expect(annotations).To(HaveKeyWithValue("io.opentelemetry.discovery.logs.pgbouncer/enabled", "true"))
			Expect(annotations).NotTo(HaveKey(ContainSubstring("9187")))
			Expect(annotations).NotTo(HaveKey(ContainSubstring("postgres")))
			Expect(parseMetrics(annotations, "9127").JobName).To(Equal("cloudnative-pg-pooler"))
		})

		It("sets the five hints and nothing else", func() {
			Expect(Annotations([]Family{OpenTelemetry}, instance)).To(HaveLen(5))
		})
	})

	It("puts both families side by side, each unchanged", func() {
		both := Annotations([]Family{Prometheus, OpenTelemetry}, instanceTLS)
		Expect(both).To(HaveLen(9))
		for key, value := range Annotations([]Family{Prometheus}, instanceTLS) {
			Expect(both).To(HaveKeyWithValue(key, value))
		}
		for key, value := range Annotations([]Family{OpenTelemetry}, instanceTLS) {
			Expect(both).To(HaveKeyWithValue(key, value))
		}
	})

	It("uses only the prefixes of the two families", func() {
		for key := range Annotations(Families, instance) {
			Expect(strings.HasPrefix(key, "prometheus.io/") ||
				strings.HasPrefix(key, "io.opentelemetry.discovery.")).To(BeTrue(), key)
		}
	})

	It("is the same at every call", func() {
		first := Annotations(Families, instanceTLS)
		for range 20 {
			Expect(Annotations(Families, instanceTLS)).To(Equal(first))
		}
	})
})
