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

package metricserver

import (
	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/cloudnative-pg/cloudnative-pg/pkg/management/postgres/volumeusage"
)

// diskMetrics are the gauges describing the filesystems of the volumes
// of the instance
type diskMetrics struct {
	TotalBytes     *prometheus.GaugeVec
	AvailableBytes *prometheus.GaugeVec
	UsedBytes      *prometheus.GaugeVec
	Inodes         *prometheus.GaugeVec
	InodesFree     *prometheus.GaugeVec
}

func newDiskMetrics() diskMetrics {
	subsystem := "disk"
	return diskMetrics{
		TotalBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: PrometheusNamespace,
			Subsystem: subsystem,
			Name:      "total_bytes",
			Help:      "Size in bytes of the filesystem of the volume",
		}, []string{"volume"}),
		AvailableBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: PrometheusNamespace,
			Subsystem: subsystem,
			Name:      "available_bytes",
			Help:      "Bytes of the filesystem of the volume available to PostgreSQL, reserved blocks excluded",
		}, []string{"volume"}),
		UsedBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: PrometheusNamespace,
			Subsystem: subsystem,
			Name:      "used_bytes",
			Help:      "Bytes used in the filesystem of the volume",
		}, []string{"volume"}),
		Inodes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: PrometheusNamespace,
			Subsystem: subsystem,
			Name:      "inodes",
			Help: "Number of inodes of the filesystem of the volume. " +
				"Not reported when the filesystem does not report inodes",
		}, []string{"volume"}),
		InodesFree: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: PrometheusNamespace,
			Subsystem: subsystem,
			Name:      "inodes_free",
			Help: "Number of free inodes of the filesystem of the volume. " +
				"Not reported when the filesystem does not report inodes",
		}, []string{"volume"}),
	}
}

func (d diskMetrics) describe(ch chan<- *prometheus.Desc) {
	d.TotalBytes.Describe(ch)
	d.AvailableBytes.Describe(ch)
	d.UsedBytes.Describe(ch)
	d.Inodes.Describe(ch)
	d.InodesFree.Describe(ch)
}

func (d diskMetrics) collect(ch chan<- prometheus.Metric) {
	d.TotalBytes.Collect(ch)
	d.AvailableBytes.Collect(ch)
	d.UsedBytes.Collect(ch)
	d.Inodes.Collect(ch)
	d.InodesFree.Collect(ch)
}

func (d diskMetrics) reset() {
	d.TotalBytes.Reset()
	d.AvailableBytes.Reset()
	d.UsedBytes.Reset()
	d.Inodes.Reset()
	d.InodesFree.Reset()
}

// updateDiskMetrics updates the disk metrics. It runs apart from the instance
// metrics, which are skipped while the instance is fenced or PostgreSQL is
// down: a full volume is one of the causes.
func (e *Exporter) updateDiskMetrics() {
	if err := collectDiskMetrics(e); err != nil {
		log.Error(err, "while collecting disk metrics")
		e.Metrics.Error.Set(1)
		e.Metrics.PgCollectionErrors.WithLabelValues("Collect.Disk").Inc()
	}
}

// collectDiskMetrics reads the filesystem of each volume of the instance.
// A volume that cannot be read is left out, and the others are reported.
func collectDiskMetrics(e *Exporter) error {
	e.Metrics.Disk.reset()

	usages, err := volumeusage.Get(e.diskPaths, e.statfs)
	for _, usage := range usages {
		e.Metrics.Disk.TotalBytes.WithLabelValues(usage.Volume).Set(float64(usage.TotalBytes))
		e.Metrics.Disk.AvailableBytes.WithLabelValues(usage.Volume).Set(float64(usage.AvailableBytes))
		e.Metrics.Disk.UsedBytes.WithLabelValues(usage.Volume).Set(float64(usage.UsedBytes))
		if usage.Inodes > 0 {
			e.Metrics.Disk.Inodes.WithLabelValues(usage.Volume).Set(float64(usage.Inodes))
			e.Metrics.Disk.InodesFree.WithLabelValues(usage.Volume).Set(float64(usage.InodesFree))
		}
	}

	return err
}
