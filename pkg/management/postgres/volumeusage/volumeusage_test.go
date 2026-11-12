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

package volumeusage

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"

	"github.com/cloudnative-pg/cloudnative-pg/pkg/postgres"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("volume usage", func() {
	var paths Paths

	BeforeEach(func() {
		root := GinkgoT().TempDir()
		paths = Paths{
			PgData:      filepath.Join(root, "data"),
			WalVolume:   filepath.Join(root, "wal"),
			Tablespaces: filepath.Join(root, "tablespaces"),
		}
		Expect(os.Mkdir(paths.PgData, 0o700)).To(Succeed())
	})

	names := func(usages []postgres.VolumeUsage) []string {
		result := make([]string, 0, len(usages))
		for _, usage := range usages {
			result = append(result, usage.Volume)
		}
		return result
	}

	It("reads PGDATA, the WAL volume and the tablespace volumes from the real filesystem", func() {
		usages, err := Get(paths, syscall.Statfs)
		Expect(err).ToNot(HaveOccurred())
		Expect(names(usages)).To(ConsistOf("pgdata"))
		Expect(usages[0].TotalBytes).To(BeNumerically(">", 0))

		Expect(os.Mkdir(paths.WalVolume, 0o700)).To(Succeed())
		Expect(os.MkdirAll(filepath.Join(paths.Tablespaces, "alpha"), 0o700)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(paths.Tablespaces, "not-a-volume"), nil, 0o600)).To(Succeed())

		usages, err = Get(paths, syscall.Statfs)
		Expect(err).ToNot(HaveOccurred())
		Expect(names(usages)).To(ConsistOf("pgdata", "wal", "tbs-alpha"))
	})

	It("computes the bytes and the inodes from the blocks, and leaves out a volume that cannot be read", func() {
		Expect(os.Mkdir(paths.WalVolume, 0o700)).To(Succeed())
		statfs := func(path string, stat *syscall.Statfs_t) error {
			if path == paths.WalVolume {
				return syscall.EIO
			}
			*stat = syscall.Statfs_t{Bsize: 4096, Blocks: 100, Bfree: 40, Bavail: 30, Files: 50, Ffree: 10}
			return nil
		}

		usages, err := Get(paths, statfs)

		Expect(err).To(MatchError(ContainSubstring("volume wal")))
		Expect(errors.Is(err, syscall.EIO)).To(BeTrue())
		Expect(usages).To(ConsistOf(postgres.VolumeUsage{
			Volume:         "pgdata",
			TotalBytes:     409600,
			AvailableBytes: 122880,
			UsedBytes:      245760,
			Inodes:         50,
			InodesFree:     10,
		}))
	})

	It("reports no inodes for a filesystem without, and does not underflow", func() {
		statfs := func(_ string, stat *syscall.Statfs_t) error {
			*stat = syscall.Statfs_t{Bsize: 4096, Blocks: 100, Bfree: 120, Bavail: 100}
			return nil
		}

		usages, err := Get(paths, statfs)

		Expect(err).ToNot(HaveOccurred())
		Expect(usages).To(ConsistOf(postgres.VolumeUsage{
			Volume: "pgdata", TotalBytes: 409600, AvailableBytes: 409600,
		}))
	})
})
