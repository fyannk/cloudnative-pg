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
	"os"
	"path/filepath"
	"syscall"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/cloudnative-pg/cloudnative-pg/internal/management/cache"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/management/postgres"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// fakeStatfs answers statfs with the given values per path, and an error for
// the paths in failing
func fakeStatfs(values map[string]syscall.Statfs_t, failing map[string]error) func(string, *syscall.Statfs_t) error {
	return func(path string, stat *syscall.Statfs_t) error {
		if err, ok := failing[path]; ok {
			return err
		}
		*stat = values[path]
		return nil
	}
}

// gaugeValue returns the value of a gauge
func gaugeValue(gauge prometheus.Metric) float64 {
	var m dto.Metric
	Expect(gauge.Write(&m)).To(Succeed())
	if m.GetCounter() != nil {
		return m.GetCounter().GetValue()
	}
	return m.GetGauge().GetValue()
}

// diskSeries returns the volume labels of the series of a disk gauge
func diskSeries(gauge *prometheus.GaugeVec) []string {
	ch := make(chan prometheus.Metric, 16)
	gauge.Collect(ch)
	close(ch)

	var volumes []string
	for metric := range ch {
		var m dto.Metric
		Expect(metric.Write(&m)).To(Succeed())
		volumes = append(volumes, m.GetLabel()[0].GetValue())
	}
	return volumes
}

var _ = Describe("disk metrics", func() {
	var (
		exporter *Exporter
		root     string
	)

	BeforeEach(func() {
		cache.Delete(cache.ClusterKey)
		exporter = NewExporter(postgres.NewInstance(), fakePluginCollector{})
		root = GinkgoT().TempDir()
		exporter.diskPaths = diskPaths{
			pgData:      filepath.Join(root, "data"),
			walVolume:   filepath.Join(root, "wal"),
			tablespaces: filepath.Join(root, "tablespaces"),
		}
		Expect(os.Mkdir(exporter.diskPaths.pgData, 0o700)).To(Succeed())
	})

	It("reports only PGDATA when there is no WAL volume and no tablespace", func() {
		Expect(collectDiskMetrics(exporter)).To(Succeed())

		Expect(diskSeries(exporter.Metrics.Disk.TotalBytes)).To(ConsistOf("pgdata"))
		Expect(gaugeValue(exporter.Metrics.Disk.TotalBytes.WithLabelValues("pgdata"))).To(BeNumerically(">", 0))
	})

	It("reports the WAL volume and one volume per tablespace, read from the real filesystem", func() {
		Expect(os.Mkdir(exporter.diskPaths.walVolume, 0o700)).To(Succeed())
		Expect(os.MkdirAll(filepath.Join(exporter.diskPaths.tablespaces, "beta"), 0o700)).To(Succeed())
		Expect(os.MkdirAll(filepath.Join(exporter.diskPaths.tablespaces, "alpha"), 0o700)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(exporter.diskPaths.tablespaces, "not-a-volume"), nil, 0o600)).To(Succeed())

		Expect(collectDiskMetrics(exporter)).To(Succeed())

		for _, gauge := range []*prometheus.GaugeVec{
			exporter.Metrics.Disk.TotalBytes,
			exporter.Metrics.Disk.AvailableBytes,
			exporter.Metrics.Disk.UsedBytes,
		} {
			Expect(diskSeries(gauge)).To(ConsistOf("pgdata", "wal", "tbs-alpha", "tbs-beta"))
		}
		total := gaugeValue(exporter.Metrics.Disk.TotalBytes.WithLabelValues("wal"))
		Expect(gaugeValue(exporter.Metrics.Disk.AvailableBytes.WithLabelValues("wal"))).To(
			BeNumerically("<=", total))
		Expect(gaugeValue(exporter.Metrics.Disk.UsedBytes.WithLabelValues("wal"))).To(
			BeNumerically("<=", total))
	})

	It("computes the bytes from the blocks and reports the inodes", func() {
		exporter.statfs = fakeStatfs(map[string]syscall.Statfs_t{
			exporter.diskPaths.pgData: {Bsize: 4096, Blocks: 100, Bfree: 40, Bavail: 30, Files: 50, Ffree: 10},
		}, nil)

		Expect(collectDiskMetrics(exporter)).To(Succeed())

		Expect(gaugeValue(exporter.Metrics.Disk.TotalBytes.WithLabelValues("pgdata"))).To(BeEquivalentTo(409600))
		Expect(gaugeValue(exporter.Metrics.Disk.UsedBytes.WithLabelValues("pgdata"))).To(BeEquivalentTo(245760))
		// the blocks reserved to root are not available to PostgreSQL
		Expect(gaugeValue(exporter.Metrics.Disk.AvailableBytes.WithLabelValues("pgdata"))).To(
			BeEquivalentTo(122880))
		Expect(gaugeValue(exporter.Metrics.Disk.Inodes.WithLabelValues("pgdata"))).To(BeEquivalentTo(50))
		Expect(gaugeValue(exporter.Metrics.Disk.InodesFree.WithLabelValues("pgdata"))).To(BeEquivalentTo(10))
	})

	It("does not report inodes for a filesystem that reports none", func() {
		exporter.statfs = fakeStatfs(map[string]syscall.Statfs_t{
			exporter.diskPaths.pgData: {Bsize: 4096, Blocks: 100, Bfree: 40, Bavail: 40},
		}, nil)

		Expect(collectDiskMetrics(exporter)).To(Succeed())

		Expect(diskSeries(exporter.Metrics.Disk.TotalBytes)).To(ConsistOf("pgdata"))
		Expect(diskSeries(exporter.Metrics.Disk.Inodes)).To(BeEmpty())
		Expect(diskSeries(exporter.Metrics.Disk.InodesFree)).To(BeEmpty())
	})

	It("does not underflow when more blocks are free than the filesystem has", func() {
		exporter.statfs = fakeStatfs(map[string]syscall.Statfs_t{
			exporter.diskPaths.pgData: {Bsize: 4096, Blocks: 100, Bfree: 120, Bavail: 100},
		}, nil)

		Expect(collectDiskMetrics(exporter)).To(Succeed())

		Expect(gaugeValue(exporter.Metrics.Disk.UsedBytes.WithLabelValues("pgdata"))).To(BeZero())
	})

	It("reports the other volumes when one cannot be read, and drops its old series", func() {
		Expect(os.Mkdir(exporter.diskPaths.walVolume, 0o700)).To(Succeed())
		values := map[string]syscall.Statfs_t{
			exporter.diskPaths.pgData:    {Bsize: 4096, Blocks: 100, Bfree: 40, Bavail: 30, Files: 50, Ffree: 10},
			exporter.diskPaths.walVolume: {Bsize: 4096, Blocks: 200, Bfree: 40, Bavail: 30, Files: 50, Ffree: 10},
		}
		exporter.statfs = fakeStatfs(values, nil)
		Expect(collectDiskMetrics(exporter)).To(Succeed())
		Expect(diskSeries(exporter.Metrics.Disk.TotalBytes)).To(ConsistOf("pgdata", "wal"))

		exporter.statfs = fakeStatfs(values, map[string]error{exporter.diskPaths.walVolume: syscall.EIO})
		err := collectDiskMetrics(exporter)

		Expect(err).To(MatchError(ContainSubstring("volume wal")))
		Expect(errors.Is(err, syscall.EIO)).To(BeTrue())
		Expect(diskSeries(exporter.Metrics.Disk.TotalBytes)).To(ConsistOf("pgdata"))
		Expect(diskSeries(exporter.Metrics.Disk.Inodes)).To(ConsistOf("pgdata"))
	})

	It("counts a failed read as a collection error", func() {
		exporter.statfs = fakeStatfs(nil, map[string]error{exporter.diskPaths.pgData: syscall.EIO})

		exporter.updateDiskMetrics()

		Expect(gaugeValue(exporter.Metrics.Error)).To(BeEquivalentTo(1))
		Expect(gaugeValue(exporter.Metrics.PgCollectionErrors.WithLabelValues("Collect.Disk"))).To(
			BeEquivalentTo(1))
	})

	It("is collected while the instance is fenced, when the other instance metrics are skipped", func() {
		exporter.instance.SetFencing(true)
		exporter.statfs = fakeStatfs(map[string]syscall.Statfs_t{
			exporter.diskPaths.pgData: {Bsize: 4096, Blocks: 100, Bfree: 100, Bavail: 100, Files: 50, Ffree: 0},
		}, nil)

		registry := prometheus.NewRegistry()
		registry.MustRegister(exporter)
		families, err := registry.Gather()
		Expect(err).ToNot(HaveOccurred())
		names := make([]string, 0, len(families))
		for _, family := range families {
			names = append(names, family.GetName())
		}
		Expect(names).To(ContainElements("cnpg_disk_total_bytes", "cnpg_disk_inodes_free", "cnpg_collector_fencing_on"))
		Expect(gaugeValue(exporter.Metrics.Disk.InodesFree.WithLabelValues("pgdata"))).To(BeZero())
		Expect(gaugeValue(exporter.Metrics.FencingOn)).To(BeEquivalentTo(1))
	})
})
