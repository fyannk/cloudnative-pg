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

package run

import (
	"os"
	"path/filepath"
	"syscall"

	"github.com/cloudnative-pg/cloudnative-pg/pkg/management/postgres/volumeusage"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("full volumes named when the instance manager exits", func() {
	var paths volumeusage.Paths

	BeforeEach(func() {
		root := GinkgoT().TempDir()
		paths = volumeusage.Paths{
			PgData:      filepath.Join(root, "data"),
			WalVolume:   filepath.Join(root, "wal"),
			Tablespaces: filepath.Join(root, "tablespaces"),
		}
		Expect(os.Mkdir(paths.PgData, 0o700)).To(Succeed())
		Expect(os.Mkdir(paths.WalVolume, 0o700)).To(Succeed())
		Expect(os.MkdirAll(filepath.Join(paths.Tablespaces, "alpha"), 0o700)).To(Succeed())

		previous := terminationLogPath
		terminationLogPath = filepath.Join(GinkgoT().TempDir(), "termination-log")
		DeferCleanup(func() { terminationLogPath = previous })
	})

	// statfs reports the given available blocks, of 4KiB, for the tablespace
	// volume, and the others half used
	statfs := func(tablespaceAvailable uint64) volumeusage.Statfs {
		//nolint:unparam // the signature of statfs
		return func(path string, stat *syscall.Statfs_t) error {
			*stat = syscall.Statfs_t{Bsize: 4096, Blocks: 262144, Bfree: 131072, Bavail: 131072}
			if filepath.Base(path) == "alpha" {
				stat.Bfree, stat.Bavail = tablespaceAvailable+10, tablespaceAvailable
			}
			return nil
		}
	}

	It("names a volume with less than a PostgreSQL block available", func(ctx SpecContext) {
		Expect(getVolumesWithoutSpace(ctx, paths, statfs(0))).To(Equal([]string{"tbs-alpha"}))
		Expect(getVolumesWithoutSpace(ctx, paths, statfs(1))).To(Equal([]string{"tbs-alpha"}))
		Expect(getVolumesWithoutSpace(ctx, paths, statfs(2))).To(BeEmpty())
	})

	It("writes the termination message, sorted and without duplicates", func(ctx SpecContext) {
		writeDiskFullTerminationMessage(ctx, "wal", "tbs-alpha", "wal")

		written, err := os.ReadFile(terminationLogPath) //nolint:gosec // a file of the test
		Expect(err).ToNot(HaveOccurred())
		Expect(string(written)).To(Equal(`{"reason":"DiskFull","volumes":["tbs-alpha","wal"]}`))
	})

	It("writes nothing when no volume is full", func(ctx SpecContext) {
		writeDiskFullTerminationMessage(ctx)
		Expect(terminationLogPath).ToNot(BeAnExistingFile())
	})
})
