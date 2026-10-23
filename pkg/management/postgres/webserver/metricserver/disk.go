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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/cloudnative-pg/cloudnative-pg/pkg/specs"
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

// diskPaths are the mount points of the volumes of the instance
type diskPaths struct {
	pgData      string
	walVolume   string
	tablespaces string
}

// defaultDiskPaths are the mount points used by the instance Pods
var defaultDiskPaths = diskPaths{
	pgData:      specs.PgDataVolumePath,
	walVolume:   specs.PgWalVolumePath,
	tablespaces: specs.PgTablespaceVolumePath,
}

// diskVolume is a volume of the instance, as reported in the "volume" label
type diskVolume struct {
	name string
	path string
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

// listDiskVolumes returns the volumes mounted in the instance: PGDATA,
// the WAL volume when there is one, and one volume per tablespace
func listDiskVolumes(paths diskPaths) ([]diskVolume, error) {
	volumes := []diskVolume{{name: "pgdata", path: paths.pgData}}
	var errs []error

	switch _, err := os.Stat(paths.walVolume); {
	case err == nil:
		volumes = append(volumes, diskVolume{name: "wal", path: paths.walVolume})
	case !errors.Is(err, os.ErrNotExist):
		errs = append(errs, err)
	}

	entries, err := os.ReadDir(paths.tablespaces)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		volumes = append(volumes, diskVolume{
			name: "tbs-" + entry.Name(),
			path: filepath.Join(paths.tablespaces, entry.Name()),
		})
	}

	return volumes, errors.Join(errs...)
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
// It only reads the mount points, so it works while PostgreSQL is down.
// A volume that cannot be read is left out, and the others are reported.
func collectDiskMetrics(e *Exporter) error {
	e.Metrics.Disk.reset()

	volumes, err := listDiskVolumes(e.diskPaths)
	errs := []error{err}
	for _, volume := range volumes {
		var stat syscall.Statfs_t
		if err := e.statfs(volume.path, &stat); err != nil {
			errs = append(errs, fmt.Errorf("volume %s: %w", volume.name, err))
			continue
		}

		blockSize := float64(stat.Bsize)
		e.Metrics.Disk.TotalBytes.WithLabelValues(volume.name).Set(float64(stat.Blocks) * blockSize)
		e.Metrics.Disk.AvailableBytes.WithLabelValues(volume.name).Set(float64(stat.Bavail) * blockSize)
		used := 0.0
		if stat.Blocks > stat.Bfree {
			used = float64(stat.Blocks-stat.Bfree) * blockSize
		}
		e.Metrics.Disk.UsedBytes.WithLabelValues(volume.name).Set(used)

		// statfs sets the fields a filesystem does not support to 0
		if stat.Files > 0 {
			e.Metrics.Disk.Inodes.WithLabelValues(volume.name).Set(float64(stat.Files))
			e.Metrics.Disk.InodesFree.WithLabelValues(volume.name).Set(float64(stat.Ffree))
		}
	}

	return errors.Join(errs...)
}
